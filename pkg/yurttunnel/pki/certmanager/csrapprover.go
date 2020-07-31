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

package certmanager

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	certificates "k8s.io/api/certificates/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	certv1beta1 "k8s.io/client-go/informers/certificates/v1beta1"
	"k8s.io/client-go/kubernetes"
	typev1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// YurttunnelCSRApprover is the controller that auto approve all
// yurttunnel related CSR
type YurttunnelCSRApprover struct {
	csrInformer certv1beta1.CertificateSigningRequestInformer
	csrClient   typev1beta1.CertificateSigningRequestInterface
	workqueue   workqueue.RateLimitingInterface
	stopCh      <-chan struct{}
}

// Run starts the YurttunnelCSRApprover
func (yca *YurttunnelCSRApprover) Run(threadiness int) {
	defer runtime.HandleCrash()
	defer yca.workqueue.ShutDown()
	go yca.csrInformer.Informer().Run(yca.stopCh)
	klog.Info("starting the crsapprover")
	if !cache.WaitForCacheSync(yca.stopCh,
		yca.csrInformer.Informer().HasSynced) {
		klog.Error("sync csr timeout")
		return
	}
	for i := 0; i < threadiness; i++ {
		go wait.Until(yca.runWorker, time.Second, yca.stopCh)
	}
	<-yca.stopCh
	klog.Info("stoping the csrapprover")
}

func (yca *YurttunnelCSRApprover) runWorker() {
	for yca.processNextItem() {
	}
}

func (yca *YurttunnelCSRApprover) processNextItem() bool {
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
		enqueueObj(yca.workqueue, csr)
	}

	if err := approveYurttunnelCSR(csr, yca.csrClient); err != nil {
		runtime.HandleError(err)
		enqueueObj(yca.workqueue, csr)
	}

	return true
}

func enqueueObj(wq workqueue.RateLimitingInterface, obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	wq.AddRateLimited(key)
}

// NewCSRApprover creates a new YurttunnelCSRApprover
func NewCSRApprover(
	clientset kubernetes.Interface,
	sharedInformerFactory informers.SharedInformerFactory,
	stopCh <-chan struct{}) *YurttunnelCSRApprover {
	csrInformer := sharedInformerFactory.Certificates().V1beta1().
		CertificateSigningRequests()
	csrClient := clientset.CertificatesV1beta1().CertificateSigningRequests()
	wq := workqueue.
		NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	csrInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			enqueueObj(wq, obj)
		},
		UpdateFunc: func(old, new interface{}) {
			enqueueObj(wq, new)
		},
	})
	return &YurttunnelCSRApprover{
		csrInformer: csrInformer,
		csrClient:   csrClient,
		workqueue:   wq,
		stopCh:      stopCh,
	}
}

// approveYurttunnelCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func approveYurttunnelCSR(
	obj interface{},
	csrClient typev1beta1.CertificateSigningRequestInterface) error {
	csr := obj.(*certificates.CertificateSigningRequest)
	if !isYurttunelCSR(csr) {
		klog.Infof("csr(%s) is not Yurttunnel csr", csr.GetName())
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

	// approve the yurttunnel related csr
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificates.CertificateSigningRequestCondition{
			Type:    certificates.CertificateApproved,
			Reason:  "AutoApproved",
			Message: "self-approving yurttunnel csr",
		})

	result, err := csrClient.UpdateApproval(csr)
	if err != nil {
		if result == nil {
			klog.Errorf("failed to approve yurttunnel csr, %v", err)
			return err
		} else {
			klog.Errorf("failed to approve yurttunnel csr(%s), %v",
				result.Name, err)
			return err
		}
	}
	klog.Infof("successfully approve yurttunnel csr(%s)", result.Name)
	return nil
}

// isYurttunelCSR checks if given csr is a yurtunnel related csr, i.e.,
// the organizations' list contains "openyurt:yurttunnel"
func isYurttunelCSR(csr *certificates.CertificateSigningRequest) bool {
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
		if org == constants.YurttunnelCSROrg {
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
