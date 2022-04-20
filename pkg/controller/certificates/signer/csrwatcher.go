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

package signer

import (
	"context"
	"fmt"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// YurtCSRApprover is the controller that auto approve all openyurt related CSR
type YurtCSRApprover struct {
	client        kubernetes.Interface
	workqueue     workqueue.RateLimitingInterface
	getCsr        func(string) (*certificatesv1.CertificateSigningRequest, error)
	hasSynced     func() bool
	signerHandler func(*certificatesv1.CertificateSigningRequest) error
}

// NewCSRApprover creates a new YurtCSRApprover
func NewCSRApprover(client kubernetes.Interface, sharedInformers informers.SharedInformerFactory, signerHandler func(*certificatesv1.CertificateSigningRequest) error) (*YurtCSRApprover, error) {
	var hasSynced func() bool
	var getCsr func(string) (*certificatesv1.CertificateSigningRequest, error)

	// init workqueue and event handler
	wq := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	yca := &YurtCSRApprover{
		client:        client,
		workqueue:     wq,
		signerHandler: signerHandler,
	}

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			yca.enqueueObj(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			yca.enqueueObj(new)
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
	yca.hasSynced = hasSynced
	yca.getCsr = getCsr

	return yca, nil
}

func (yca *YurtCSRApprover) enqueueObj(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}

	yca.workqueue.AddRateLimited(key)
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
	klog.Infof("csr synced")
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
	defer yca.workqueue.Done(key)

	if err := yca.syncFunc(key.(string)); err != nil {
		yca.workqueue.AddRateLimited(key)
		runtime.HandleError(fmt.Errorf("sync csr %v failed with : %v", key, err))
		return true
	}

	yca.workqueue.Forget(key)
	return true
}

func (yca *YurtCSRApprover) syncFunc(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing certificate request %q (%v)", key, time.Since(startTime))
	}()

	csr, err := yca.getCsr(key)
	if apierrors.IsNotFound(err) {
		klog.V(3).Infof("csr has been deleted: %v", key)
		return nil
	}
	if err != nil {
		return err
	}

	if len(csr.Status.Certificate) > 0 {
		// no need to do anything because it already has a cert
		return nil
	}

	// need to operate on a copy so we don't mutate the csr in the shared cache
	csr = csr.DeepCopy()
	return yca.handlerCSR(csr)
}

// approveCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func (yca *YurtCSRApprover) handlerCSR(csr *certificatesv1.CertificateSigningRequest) error {

	klog.Infof("process csr: %s", csr.Name)
	if len(csr.Status.Certificate) != 0 {
		return nil
	}

	if err := yca.signerHandler(csr); err != nil {
		return err
	}
	return nil
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
