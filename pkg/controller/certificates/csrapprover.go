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
	"time"

	certificates "k8s.io/api/certificates/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	certinformer "k8s.io/client-go/informers/certificates/v1beta1"
	certv1beta1 "k8s.io/client-go/informers/certificates/v1beta1"
	"k8s.io/client-go/kubernetes"
	typev1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/pki/certmanager"
)

const (
	YurtHubCSRApproverThreadiness = 2
)

// YurtHubCSRApprover is the controller that auto approve all
// yurthub related CSR
type YurtHubCSRApprover struct {
	csrInformer certv1beta1.CertificateSigningRequestInformer
	csrClient   typev1beta1.CertificateSigningRequestInterface
	workqueue   workqueue.RateLimitingInterface
}

// Run starts the YurtHubCSRApprover
func (yca *YurtHubCSRApprover) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer yca.workqueue.ShutDown()
	klog.Info("starting the crsapprover")
	if !cache.WaitForCacheSync(stopCh,
		yca.csrInformer.Informer().HasSynced) {
		klog.Error("sync csr timeout")
		return
	}
	for i := 0; i < threadiness; i++ {
		go wait.Until(yca.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.Info("stoping the csrapprover")
}

func (yca *YurtHubCSRApprover) runWorker() {
	for yca.processNextItem() {
	}
}

func (yca *YurtHubCSRApprover) processNextItem() bool {
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

	csr, err := yca.csrInformer.Lister().Get(csrName)
	if err != nil {
		runtime.HandleError(err)
		if !apierrors.IsNotFound(err) {
			yca.workqueue.AddRateLimited(key)
		}
		return true
	}

	if err := approveYurtHubCSR(csr, yca.csrClient); err != nil {
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

	csr, ok := obj.(*certificates.CertificateSigningRequest)
	if !ok {
		klog.Errorf("%s is not a csr", key)
		return
	}

	if !isYurtHubCSR(csr) {
		klog.Infof("csr(%s) is not %s csr", csr.GetName(), projectinfo.GetHubName())
		return
	}

	approved, denied := checkCertApprovalCondition(&csr.Status)
	if !approved && !denied {
		klog.Infof("non-approved and non-denied csr, enqueue: %s", key)
		wq.AddRateLimited(key)
	}

	klog.V(4).Infof("approved or denied csr, ignore it: %s", key)
}

// NewCSRApprover creates a new YurtHubCSRApprover
func NewCSRApprover(
	clientset kubernetes.Interface,
	csrInformer certinformer.CertificateSigningRequestInformer) *YurtHubCSRApprover {

	wq := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	csrInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			enqueueObj(wq, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			enqueueObj(wq, new)
		},
	})
	return &YurtHubCSRApprover{
		csrInformer: csrInformer,
		csrClient:   clientset.CertificatesV1beta1().CertificateSigningRequests(),
		workqueue:   wq,
	}
}

// approveYurtHubCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func approveYurtHubCSR(
	obj interface{},
	csrClient typev1beta1.CertificateSigningRequestInterface) error {
	csr, ok := obj.(*certificates.CertificateSigningRequest)
	if !ok {
		return nil
	}

	if !isYurtHubCSR(csr) {
		klog.Infof("csr(%s) is not %s csr", csr.GetName(), projectinfo.GetHubName())
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

	// approve the yurthub related csr
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificates.CertificateSigningRequestCondition{
			Type:    certificates.CertificateApproved,
			Reason:  "AutoApproved",
			Message: fmt.Sprintf("self-approving %s csr", projectinfo.GetHubName()),
		})

	result, err := csrClient.UpdateApproval(context.Background(), csr, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("failed to approve %s csr(%s), %v", projectinfo.GetHubName(), csr.GetName(), err)
		return err
	}
	klog.Infof("successfully approve %s csr(%s)", projectinfo.GetHubName(), result.Name)
	return nil
}

// isYurtHubCSR checks if given csr is a yurthub related csr, i.e.,
// the organizations' list contains "openyurt:yurthub"
func isYurtHubCSR(csr *certificates.CertificateSigningRequest) bool {
	pemBytes := csr.Spec.Request
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false
	}
	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false
	}
	for i, org := range x509cr.Subject.Organization {
		if org == certmanager.YurtHubCSROrg {
			break
		}
		if i == len(x509cr.Subject.Organization)-1 {
			return false
		}
	}
	return true
}

// checkCertApprovalCondition checks if the given csr's status is
// approved or denied
func checkCertApprovalCondition(
	status *certificates.CertificateSigningRequestStatus) (
	approved bool, denied bool) {
	for _, c := range status.Conditions {
		if c.Type == certificates.CertificateApproved {
			approved = true
		}
		if c.Type == certificates.CertificateDenied {
			denied = true
		}
	}
	return
}
