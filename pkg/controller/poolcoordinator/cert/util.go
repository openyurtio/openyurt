/*
Copyright 2022 The OpenYurt Authors.

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

package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog/v2"
)

// a simple client to handle secret operations
type SecretClient struct {
	Name      string
	Namespace string
	client    client.Interface
}

func NewSecretClient(clientSet client.Interface, ns, name string) (*SecretClient, error) {

	emptySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data:       make(map[string][]byte),
		StringData: make(map[string]string),
	}

	secret, err := clientSet.CoreV1().Secrets(ns).Create(context.TODO(), emptySecret, metav1.CreateOptions{})
	if err != nil {
		// because multiple SecretClient may share one secret
		// if this secret already exist, reuse it
		if kerrors.IsAlreadyExists(err) {
			secret, _ = clientSet.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
			klog.Infof("secret %s already exisit: %v", name, secret)
		} else {
			return nil, fmt.Errorf("create secret client %s fail: %v", name, err)
		}
	} else {
		klog.Infof("secret %s not exisit, create one: %v", name, secret)
	}

	return &SecretClient{
		Name:      name,
		Namespace: ns,
		client:    clientSet,
	}, nil
}

func (c *SecretClient) AddData(key string, val []byte) error {

	patchBytes, _ := json.Marshal(map[string]interface{}{"data": map[string][]byte{key: val}})
	_, err := c.client.CoreV1().Secrets(c.Namespace).Patch(context.TODO(), c.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})

	if err != nil {
		return fmt.Errorf("update secret %v/%v %s fail: %v", c.Namespace, c.Name, key, err)
	}

	return nil
}

// EncodeCertPEM returns PEM-endcoded certificate data
func MarshalCertToPEM(c *tls.Certificate) ([]byte, error) {

	var x509Cert *x509.Certificate

	if c.Leaf != nil {
		x509Cert = c.Leaf
	}
	x509Cert, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, errors.Wrapf(err, "parse x509 certificate fail")
	}

	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: x509Cert.Raw,
	}
	return pem.EncodeToMemory(&block), nil
}

func GetCertFromTLSCert(cert *tls.Certificate) (certPEM []byte, err error) {
	if cert == nil {
		return nil, errors.New("tls certificate cannot be nil")
	}

	return MarshalCertToPEM(cert)
}

func GetPrivateKeyFromTLSCert(cert *tls.Certificate) (keyPEM []byte, err error) {
	if cert == nil {
		return nil, errors.New("tls certificate cannot be nil")
	}

	return keyutil.MarshalPrivateKeyToPEM(cert.PrivateKey)
}

func GetURLFromSVC(svc *corev1.Service) (string, error) {
	hostName := svc.Spec.ClusterIP
	if svc.Spec.Ports == nil || len(svc.Spec.Ports) == 0 {
		return "", errors.New("Service port list cannot be empty")
	}
	port := svc.Spec.Ports[0].Port
	return fmt.Sprintf("https://%s:%d", hostName, port), nil
}

// get certificate & private key (in PEM format) from certmanager
func GetCertAndKeyFromCertMgr(certManager certificate.Manager, stopCh <-chan struct{}) (key []byte, cert []byte, err error) {
	// waiting for the certificate is generated
	certManager.Start()

	err = wait.PollUntil(5*time.Second, func() (bool, error) {
		// keep polling until the certificate is signed
		if certManager.Current() != nil {
			klog.Infof("%s certificate signed successfully", ComponentName)
			return true, nil
		}
		klog.Infof("waiting for the master to sign the %s certificate", ComponentName)
		return false, nil
	}, stopCh)

	if err != nil {
		return nil, nil, err
	}

	// When CSR is issued and approved
	// get key from certificate
	key, err = GetPrivateKeyFromTLSCert(certManager.Current())
	if err != nil {
		return
	}
	// get certificate from certificate
	cert, err = GetCertFromTLSCert(certManager.Current())
	if err != nil {
		return
	}

	return
}

// write cert&key pair generated from certManager into a secret
func WriteCertIntoSecret(clientSet client.Interface, certName, secretName string, certManager certificate.Manager, stopCh <-chan struct{}) error {

	keyPEM, certPEM, err := GetCertAndKeyFromCertMgr(certManager, stopCh)
	if err != nil {
		return errors.Wrapf(err, "write cert %s fail", certName)
	}

	// write certificate data into secret
	secretClient, err := NewSecretClient(clientSet, PoolcoordinatorNS, secretName)
	if err != nil {
		return err
	}
	err = secretClient.AddData(fmt.Sprintf("%s.key", certName), keyPEM)
	if err != nil {
		return err
	}
	err = secretClient.AddData(fmt.Sprintf("%s.crt", certName), certPEM)
	if err != nil {
		return err
	}

	klog.Infof("successfully write %s cert/key pair into %s", certName, secretName)

	return nil
}
