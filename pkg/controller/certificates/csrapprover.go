/*
Copyright 2020 The OpenYurt Authors.

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

package certificates

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

const (
	YurthubCSROrg              = "openyurt:yurthub"    // yurthub PKI related constants
	YurttunnelCSROrg           = "openyurt:yurttunnel" // yurttunnel PKI related constants
	YurtCSRApproverThreadiness = 2
)

var (
	yurtCsr = fmt.Sprintf("%s-csr", strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
)

// YurtCSRApprover is the controller that auto approve all openyurt related CSR
type YurtCSRApprover struct {
	client    kubernetes.Interface
	workqueue workqueue.RateLimitingInterface
	getCsr    func(string) (*certificatesv1.CertificateSigningRequest, error)
	hasSynced func() bool
}

// Run starts the YurtCSRApprover
func (yca *YurtCSRApprover) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer yca.workqueue.ShutDown()
	klog.Info("starting the crsapprover")
	if !cache.WaitForCacheSync(stopCh, yca.hasSynced) {
		klog.Error("sync csr timeout")
		return
	}
	for i := 0; i < threadiness; i++ {
		go wait.Until(yca.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.Info("stopping the csrapprover")
}

func (yca *YurtCSRApprover) runWorker() {
	for yca.processNextItem() {
	}
}

func (yca *YurtCSRApprover) processNextItem() bool {
	key, quit := yca.workqueue.Get()
	if quit {
		return false
	}
	csrName, ok := key.(string)
	if !ok {
		yca.workqueue.Forget(key)
		runtime.HandleError(
			fmt.Errorf("expected string in workqueue but got %#v", key))
		return true
	}
	defer yca.workqueue.Done(key)

	csr, err := yca.getCsr(csrName)
	if err != nil {
		runtime.HandleError(err)
		if !apierrors.IsNotFound(err) {
			yca.workqueue.AddRateLimited(key)
		}
		return true
	}

	if err := approveCSR(yca.client, csr); err != nil {
		runtime.HandleError(err)
		enqueueObj(yca.workqueue, csr)
		return true
	}

	return true
}

func enqueueObj(wq workqueue.RateLimitingInterface, obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}

	var v1Csr *certificatesv1.CertificateSigningRequest
	switch csr := obj.(type) {
	case *certificatesv1.CertificateSigningRequest:
		v1Csr = csr
	case *certificatesv1beta1.CertificateSigningRequest:
		v1Csr = v1beta1Csr2v1Csr(csr)
	default:
		klog.Errorf("%s is not a csr", key)
		return
	}

	if !isYurtCSR(v1Csr) {
		klog.Infof("csr(%s) is not %s", v1Csr.GetName(), yurtCsr)
		return
	}

	approved, denied := checkCertApprovalCondition(&v1Csr.Status)
	if !approved && !denied {
		klog.Infof("non-approved and non-denied csr, enqueue: %s", key)
		wq.AddRateLimited(key)
		return
	}

	klog.V(4).Infof("approved or denied csr, ignore it: %s", key)
}

// NewCSRApprover creates a new YurtCSRApprover
func NewCSRApprover(client kubernetes.Interface, sharedInformers informers.SharedInformerFactory) (*YurtCSRApprover, error) {
	var hasSynced func() bool
	var getCsr func(string) (*certificatesv1.CertificateSigningRequest, error)

	// init workqueue and event handler
	wq := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			enqueueObj(wq, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			enqueueObj(wq, new)
		},
	}

	// init csr synced and get handler
	_, err := client.CertificatesV1().CertificateSigningRequests().List(context.TODO(), metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		// v1.CertificateSigningRequest api is supported
		klog.Infof("v1.CertificateSigningRequest is supported.")
		sharedInformers.Certificates().V1().CertificateSigningRequests().Informer().AddEventHandler(handler)
		hasSynced = sharedInformers.Certificates().V1().CertificateSigningRequests().Informer().HasSynced
		getCsr = sharedInformers.Certificates().V1().CertificateSigningRequests().Lister().Get
	} else {
		// apierrors.IsNotFound(err), try to use v1beta1.CertificateSigningRequest api
		klog.Infof("fall back to v1beta1.CertificateSigningRequest.")
		sharedInformers.Certificates().V1beta1().CertificateSigningRequests().Informer().AddEventHandler(handler)
		hasSynced = sharedInformers.Certificates().V1beta1().CertificateSigningRequests().Informer().HasSynced
		getCsr = func(name string) (*certificatesv1.CertificateSigningRequest, error) {
			v1beta1Csr, err := sharedInformers.Certificates().V1beta1().CertificateSigningRequests().Lister().Get(name)
			if err != nil {
				return nil, err
			}
			return v1beta1Csr2v1Csr(v1beta1Csr), nil
		}
	}

	return &YurtCSRApprover{
		client:    client,
		workqueue: wq,
		getCsr:    getCsr,
		hasSynced: hasSynced,
	}, nil
}

// approveCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func approveCSR(client kubernetes.Interface, csr *certificatesv1.CertificateSigningRequest) error {
	if !isYurtCSR(csr) {
		klog.Infof("csr(%s) is not %s", csr.GetName(), yurtCsr)
		return nil
	}

	approved, denied := checkCertApprovalCondition(&csr.Status)
	if approved {
		klog.V(4).Infof("csr(%s) is approved", csr.GetName())
		return nil
	}

	if denied {
		klog.V(4).Infof("csr(%s) is denied", csr.GetName())
		return nil
	}

	// approve the openyurt related csr
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateApproved,
			Status:  corev1.ConditionTrue,
			Reason:  "AutoApproved",
			Message: fmt.Sprintf("self-approving %s", yurtCsr),
		})

	err := updateApproval(context.Background(), client, csr)
	if err != nil {
		klog.Errorf("failed to approve %s(%s), %v", yurtCsr, csr.GetName(), err)
		return err
	}
	klog.Infof("successfully approve %s(%s)", yurtCsr, csr.GetName())
	return nil
}

// isYurtCSR checks if given csr is a openyurt related csr, i.e.,
// the organizations' list contains "openyurt:yurthub"
func isYurtCSR(csr *certificatesv1.CertificateSigningRequest) bool {
	pemBytes := csr.Spec.Request
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false
	}
	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false
	}
	for _, org := range x509cr.Subject.Organization {
		if org == YurttunnelCSROrg || org == YurthubCSROrg {
			return true
		}
	}
	return false
}

// checkCertApprovalCondition checks if the given csr's status is
// approved or denied
func checkCertApprovalCondition(status *certificatesv1.CertificateSigningRequestStatus) (approved bool, denied bool) {
	for _, c := range status.Conditions {
		if c.Type == certificatesv1.CertificateApproved {
			approved = true
		}
		if c.Type == certificatesv1.CertificateDenied {
			denied = true
		}
	}
	return
}

func updateApproval(ctx context.Context, client kubernetes.Interface, csr *certificatesv1.CertificateSigningRequest) error {
	_, v1err := client.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{})
	if v1err == nil || !apierrors.IsNotFound(v1err) {
		return v1err
	}

	v1beta1Csr := v1Csr2v1beta1Csr(csr)
	_, v1beta1err := client.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(ctx, v1beta1Csr, metav1.UpdateOptions{})
	return v1beta1err
}

func v1Csr2v1beta1Csr(csr *certificatesv1.CertificateSigningRequest) *certificatesv1beta1.CertificateSigningRequest {
	v1beata1Csr := &certificatesv1beta1.CertificateSigningRequest{
		ObjectMeta: csr.ObjectMeta,
		Spec: certificatesv1beta1.CertificateSigningRequestSpec{
			Request:    csr.Spec.Request,
			SignerName: &csr.Spec.SignerName,
			Usages:     make([]certificatesv1beta1.KeyUsage, 0),
		},
		Status: certificatesv1beta1.CertificateSigningRequestStatus{
			Conditions: make([]certificatesv1beta1.CertificateSigningRequestCondition, 0),
		},
	}

	for _, usage := range csr.Spec.Usages {
		v1beata1Csr.Spec.Usages = append(v1beata1Csr.Spec.Usages, certificatesv1beta1.KeyUsage(usage))
	}

	for _, cond := range csr.Status.Conditions {
		v1beata1Csr.Status.Conditions = append(v1beata1Csr.Status.Conditions, certificatesv1beta1.CertificateSigningRequestCondition{
			Type:               certificatesv1beta1.RequestConditionType(cond.Type),
			Status:             cond.Status,
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastUpdateTime:     cond.LastUpdateTime,
			LastTransitionTime: cond.LastTransitionTime,
		})
	}

	return v1beata1Csr
}

func v1beta1Csr2v1Csr(csr *certificatesv1beta1.CertificateSigningRequest) *certificatesv1.CertificateSigningRequest {
	v1Csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: csr.ObjectMeta,
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    csr.Spec.Request,
			SignerName: *csr.Spec.SignerName,
			Usages:     make([]certificatesv1.KeyUsage, 0),
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: make([]certificatesv1.CertificateSigningRequestCondition, 0),
		},
	}

	for _, usage := range csr.Spec.Usages {
		v1Csr.Spec.Usages = append(v1Csr.Spec.Usages, certificatesv1.KeyUsage(usage))
	}

	for _, cond := range csr.Status.Conditions {
		v1Csr.Status.Conditions = append(v1Csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
			Type:               certificatesv1.RequestConditionType(cond.Type),
			Status:             cond.Status,
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastUpdateTime:     cond.LastUpdateTime,
			LastTransitionTime: cond.LastTransitionTime,
		})
	}

	return v1Csr
}
