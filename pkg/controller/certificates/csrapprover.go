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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

var (
	yurtCsr                  = fmt.Sprintf("%s-csr", strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
	yurtHubNodeCertOrgPrefix = "openyurt:tenant:"
	clientRequiredUsages     = sets.NewString(
		string(certificatesv1.UsageDigitalSignature),
		string(certificatesv1.UsageKeyEncipherment),
		string(certificatesv1.UsageClientAuth))

	serverRequiredUsages = sets.NewString(
		string(certificatesv1.UsageDigitalSignature),
		string(certificatesv1.UsageKeyEncipherment),
		string(certificatesv1.UsageServerAuth))

	recognizers = []csrRecognizer{
		{
			recognize:  isYurtHubNodeCert,
			successMsg: "Auto approving yurthub node client certificate",
		},
		{
			recognize:  isYurtTLSServerCert,
			successMsg: "Auto approving openyurt tls server certificate",
		},
		{
			recognize:  isYurtTunnelProxyClientCert,
			successMsg: "Auto approving yurt-tunnel-server proxy client certificate",
		},
		{
			recognize:  isYurtTunnelAgentCert,
			successMsg: "Auto approving yurt-tunnel-agent client certificate",
		},
	}
)

type csrRecognizer struct {
	recognize  func(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool
	successMsg string
}

// YurtCSRApprover is the controller that auto approve all openyurt related CSR
type YurtCSRApprover struct {
	client    kubernetes.Interface
	workqueue workqueue.RateLimitingInterface
	getCsr    func(string) (*certificatesv1.CertificateSigningRequest, error)
	hasSynced func() bool
}

// NewCSRApprover creates a new YurtCSRApprover
func NewCSRApprover(client kubernetes.Interface, sharedInformers informers.SharedInformerFactory) (*YurtCSRApprover, error) {
	var hasSynced func() bool
	var getCsr func(string) (*certificatesv1.CertificateSigningRequest, error)

	// init workqueue and event handler
	wq := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	yca := &YurtCSRApprover{
		client:    client,
		workqueue: wq,
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

	approved, denied := checkCertApprovalCondition(&v1Csr.Status)
	if !approved && !denied {
		klog.Infof("non-approved and non-denied csr, enqueue: %s", key)
		yca.workqueue.AddRateLimited(key)
		return
	}

	if ok, _ := isYurtCSR(v1Csr); !ok {
		klog.Infof("csr(%s) is not %s", v1Csr.GetName(), yurtCsr)
		return
	}

	klog.V(4).Infof("approved or denied csr, ignore it: %s", key)
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
	return yca.approveCSR(csr)
}

// approveCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func (yca *YurtCSRApprover) approveCSR(csr *certificatesv1.CertificateSigningRequest) error {
	if len(csr.Status.Certificate) != 0 {
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

	ok, successMsg := isYurtCSR(csr)
	if !ok {
		klog.Infof("csr(%s) is not %s", csr.GetName(), yurtCsr)
		return nil
	}

	// approve the openyurt related csr
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateApproved,
			Status:  corev1.ConditionTrue,
			Reason:  "AutoApproved",
			Message: successMsg,
		})

	err := yca.updateApproval(context.Background(), csr)
	if err != nil {
		klog.Errorf("failed to approve %s(%s), %v", yurtCsr, csr.GetName(), err)
		return err
	}
	klog.Infof("successfully approve %s(%s)", yurtCsr, csr.GetName())
	return nil
}

func (yca *YurtCSRApprover) updateApproval(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) error {
	_, v1err := yca.client.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{})
	if v1err == nil || !apierrors.IsNotFound(v1err) {
		return v1err
	}

	v1beta1Csr := v1Csr2v1beta1Csr(csr)
	_, v1beta1err := yca.client.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(ctx, v1beta1Csr, metav1.UpdateOptions{})
	return v1beta1err
}

// isYurtCSR checks if given csr is an openyurt related csr and
// return success message for specified recognizers.
func isYurtCSR(csr *certificatesv1.CertificateSigningRequest) (bool, string) {
	var successMsg string
	pemBytes := csr.Spec.Request
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false, successMsg
	}
	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false, successMsg
	}

	for _, r := range recognizers {
		if r.recognize(csr, x509cr) {
			successMsg = r.successMsg
			return true, successMsg
		}
	}

	return false, successMsg
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
	if csr == nil {
		return nil
	}
	v1Csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: csr.ObjectMeta,
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: csr.Spec.Request,
			Usages:  make([]certificatesv1.KeyUsage, 0),
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: make([]certificatesv1.CertificateSigningRequestCondition, 0),
		},
	}

	if csr.Spec.SignerName != nil {
		v1Csr.Spec.SignerName = *csr.Spec.SignerName
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

// isYurtTLSServerCert is used to recognize csr from yurthub https server that listens requests from edge clients like kubelet/kube-proxy.
// or from yurt-tunnel-server that listens requests from cloud components like kube-apiserver and prometheus or from yurt-tunnel-agent
func isYurtTLSServerCert(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if csr.Spec.SignerName != certificatesv1.KubeletServingSignerName {
		return false
	}

	if len(x509cr.Subject.Organization) != 1 || x509cr.Subject.Organization[0] != user.NodesGroup {
		return false
	}

	// at least one of dnsNames or ipAddresses must be specified
	if len(x509cr.DNSNames) == 0 && len(x509cr.IPAddresses) == 0 {
		return false
	}

	if !strings.HasPrefix(x509cr.Subject.CommonName, "system:node:") {
		return false
	}

	if !serverRequiredUsages.Equal(usagesToSet(csr.Spec.Usages)) {
		return false
	}

	return true
}

// isYurtHubNodeCert is used to recognize csr for yurthub client that forwards edge side requests to kube-apiserver
func isYurtHubNodeCert(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if csr.Spec.SignerName != certificatesv1.KubeAPIServerClientSignerName {
		return false
	}

	if len(x509cr.Subject.Organization) < 2 {
		return false
	} else {
		for _, org := range x509cr.Subject.Organization {
			if org != token.YurtHubCSROrg && org != user.NodesGroup && !strings.HasPrefix(org, yurtHubNodeCertOrgPrefix) {
				return false
			}
		}
	}

	if !strings.HasPrefix(x509cr.Subject.CommonName, "system:node:") {
		return false
	}

	if !clientRequiredUsages.Equal(usagesToSet(csr.Spec.Usages)) {
		return false
	}

	return true
}

// isYurtTunnelProxyClientCert is used to recognize csr from yurt-tunnel-server that used for proxying requests to edge nodes.
func isYurtTunnelProxyClientCert(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if csr.Spec.SignerName != certificatesv1.KubeAPIServerClientSignerName {
		return false
	}

	if len(x509cr.Subject.Organization) != 1 || x509cr.Subject.Organization[0] != constants.YurtTunnelCSROrg {
		return false
	}

	if x509cr.Subject.CommonName != constants.YurtTunnelProxyClientCSRCN {
		return false
	}

	if !clientRequiredUsages.Equal(usagesToSet(csr.Spec.Usages)) {
		return false
	}

	return true
}

// isYurtTunnelAgentCert is used to recognize csr for yurt-tunnel-agent component
func isYurtTunnelAgentCert(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if csr.Spec.SignerName != certificatesv1.KubeAPIServerClientSignerName {
		return false
	}

	if len(x509cr.Subject.Organization) != 1 || x509cr.Subject.Organization[0] != constants.YurtTunnelCSROrg {
		return false
	}

	if x509cr.Subject.CommonName != constants.YurtTunnelAgentCSRCN {
		return false
	}

	if !clientRequiredUsages.Equal(usagesToSet(csr.Spec.Usages)) {
		return false
	}

	return true
}

func usagesToSet(usages []certificatesv1.KeyUsage) sets.String {
	result := sets.NewString()
	for _, usage := range usages {
		result.Insert(string(usage))
	}
	return result
}
