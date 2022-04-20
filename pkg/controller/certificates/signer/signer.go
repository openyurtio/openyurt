/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package signer implements a CA signer that uses keys stored on local disk.
package signer

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	capi "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/pkg/controller/certificates/authority"
	"github.com/openyurtio/openyurt/pkg/util/certmanager"
)

type CSRSigningController struct {
	certificateController *YurtCSRApprover
	dynamicCertReloader   dynamiccertificates.ControllerRunner
}

func NewKubeAPIServerClientCSRSigningController(
	client clientset.Interface,
	sharedInformers informers.SharedInformerFactory,
	caFile, caKeyFile string,
	certTTL time.Duration,
) (*CSRSigningController, error) {
	return NewCSRSigningController("csrsigning-openyurt", certmanager.YurtTunnelSignerName, client, sharedInformers, caFile, caKeyFile, certTTL)
}

func NewCSRSigningController(
	controllerName string,
	signerName string,
	client clientset.Interface,
	sharedInformers informers.SharedInformerFactory,
	caFile, caKeyFile string,
	certTTL time.Duration,
) (*CSRSigningController, error) {
	signer, err := newSigner(signerName, caFile, caKeyFile, client, certTTL)
	if err != nil {
		return nil, err
	}

	csrcontroller, err := NewCSRApprover(client, sharedInformers, signer.handle)
	if err != nil {
		return nil, err
	}
	return &CSRSigningController{
		certificateController: csrcontroller,
		dynamicCertReloader:   signer.caProvider.caLoader,
	}, nil
}

// Run the main goroutine responsible for watching and syncing jobs.
func (c *CSRSigningController) Run(workers int, stopCh <-chan struct{}) {
	go c.dynamicCertReloader.Run(workers, stopCh)

	c.certificateController.Run(workers, stopCh)
}

type isRequestForSignerFunc func(req *x509.CertificateRequest, usages []capi.KeyUsage, signerName string) (bool, error)

type signer struct {
	caProvider *caProvider

	client  clientset.Interface
	certTTL time.Duration

	signerName           string
	isRequestForSignerFn isRequestForSignerFunc
}

func newSigner(signerName, caFile, caKeyFile string, client clientset.Interface, certificateDuration time.Duration) (*signer, error) {
	isRequestForSignerFn, err := getCSRVerificationFuncForSignerName(signerName)
	if err != nil {
		return nil, err
	}
	caProvider, err := newCAProvider(caFile, caKeyFile)
	if err != nil {
		return nil, err
	}

	ret := &signer{
		caProvider:           caProvider,
		client:               client,
		certTTL:              certificateDuration,
		signerName:           signerName,
		isRequestForSignerFn: isRequestForSignerFn,
	}
	return ret, nil
}

func (s *signer) handle(csr *capi.CertificateSigningRequest) error {
	// Ignore unapproved or failed requests
	if !IsCertificateRequestApproved(csr) || HasTrueCondition(csr, capi.CertificateFailed) {
		return nil
	}

	// Fast-path to avoid any additional processing if the CSRs signerName does not match
	if csr.Spec.SignerName != s.signerName {
		return nil
	}

	x509cr, err := ParseCSR(csr.Spec.Request)
	if err != nil {
		return fmt.Errorf("unable to parse csr %q: %v", csr.Name, err)
	}
	if recognized, err := s.isRequestForSignerFn(x509cr, csr.Spec.Usages, csr.Spec.SignerName); err != nil {
		csr.Status.Conditions = append(csr.Status.Conditions, capi.CertificateSigningRequestCondition{
			Type:           capi.CertificateFailed,
			Status:         v1.ConditionTrue,
			Reason:         "SignerValidationFailure",
			Message:        err.Error(),
			LastUpdateTime: metav1.Now(),
		})
		_, err = s.client.CertificatesV1().CertificateSigningRequests().UpdateStatus(context.TODO(), csr, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("error adding failure condition for csr: %v", err)
		}
		return nil
	} else if !recognized {
		// Ignore requests for kubernetes.io signerNames we don't recognize
		return nil
	}
	cert, err := s.sign(x509cr, csr.Spec.Usages)
	if err != nil {
		return fmt.Errorf("error auto signing csr: %v", err)
	}
	csr.Status.Certificate = cert
	_, v1err := s.client.CertificatesV1().CertificateSigningRequests().UpdateStatus(context.TODO(), csr, metav1.UpdateOptions{})
	if v1err == nil || !apierrors.IsNotFound(v1err) {
		return v1err
	}

	v1beta1Csr := v1Csr2v1beta1Csr(csr)
	_, v1beta1err := s.client.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(context.TODO(), v1beta1Csr, metav1.UpdateOptions{})
	if v1beta1err == nil || apierrors.IsNotFound(v1beta1err) {
		return nil
	}
	return fmt.Errorf("error updating signature for csr: %v", v1err)
}

func (s *signer) sign(x509cr *x509.CertificateRequest, usages []capi.KeyUsage) ([]byte, error) {
	currCA, err := s.caProvider.currentCA()
	if err != nil {
		return nil, err
	}
	der, err := currCA.Sign(x509cr.Raw, authority.PermissiveSigningPolicy{
		TTL:    s.certTTL,
		Usages: usages,
	})
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

func getCSRVerificationFuncForSignerName(signerName string) (isRequestForSignerFunc, error) {
	switch signerName {
	case certmanager.YurtTunnelSignerName:
		return isOpenYurtTunnel, nil
	default:
		return nil, fmt.Errorf("unrecognized signerName: %q", signerName)
	}
}

func isOpenYurtTunnel(req *x509.CertificateRequest, usages []capi.KeyUsage, signerName string) (bool, error) {
	if signerName != certmanager.YurtTunnelSignerName {
		return false, nil
	}
	return true, nil
}
