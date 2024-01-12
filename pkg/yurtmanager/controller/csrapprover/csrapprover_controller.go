/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package csrapprover

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	yurtcoorrdinatorCert "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/cert"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

var (
	csrV1Resource = certificatesv1.SchemeGroupVersion.WithResource("certificatesigningrequests")

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
			successMsg: "Auto approving tunnel-server proxy client certificate",
		},
		{
			recognize:  isYurtTunnelAgentCert,
			successMsg: "Auto approving tunnel-agent client certificate",
		},
		{
			recognize:  isYurtCoordinatorClientCert,
			successMsg: "Auto approving yurtcoordinator-apiserver client certificate",
		},
	}
)

type csrRecognizer struct {
	recognize  func(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool
	successMsg string
}

// Add creates a new CsrApprover Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(_ context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := &ReconcileCsrApprover{}
	// Create a new controller
	c, err := controller.New(names.CsrApproverController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.Config.ComponentConfig.CsrApproverController.ConcurrentCsrApproverWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for csr changes
	if r.csrV1Supported {
		return c.Watch(&source.Kind{Type: &certificatesv1.CertificateSigningRequest{}}, &handler.EnqueueRequestForObject{})
	} else {
		return c.Watch(&source.Kind{Type: &certificatesv1beta1.CertificateSigningRequest{}}, &handler.EnqueueRequestForObject{})
	}
}

var _ reconcile.Reconciler = &ReconcileCsrApprover{}

// ReconcileCsrApprover reconciles a CsrApprover object
type ReconcileCsrApprover struct {
	client.Client
	csrV1Supported    bool
	csrApproverClient kubernetes.Interface
}

func (r *ReconcileCsrApprover) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

func (r *ReconcileCsrApprover) InjectMapper(mapper meta.RESTMapper) error {
	if gvk, err := mapper.KindFor(csrV1Resource); err != nil {
		klog.Errorf("v1.CertificateSigningRequest is not supported, %v", err)
		r.csrV1Supported = false
	} else {
		klog.Infof("%s is supported", gvk.String())
		r.csrV1Supported = true
	}

	return nil
}

func (r *ReconcileCsrApprover) InjectConfig(cfg *rest.Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("could not create kube client, %v", err)
		return err
	}
	r.csrApproverClient = client
	return nil
}

// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resourceNames=kubernetes.io/kube-apiserver-client;kubernetes.io/kubelet-serving,resources=signers,verbs=approve

// Reconcile reads that state of the cluster for a CertificateSigningRequest object and makes changes based on the state read
// and what is in the CertificateSigningRequest.Spec
func (r *ReconcileCsrApprover) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CertificateSigningRequest instance
	var err error
	v1Instance := &certificatesv1.CertificateSigningRequest{}
	if r.csrV1Supported {
		err = r.Get(ctx, request.NamespacedName, v1Instance)
	} else {
		v1beta1Instance := &certificatesv1beta1.CertificateSigningRequest{}
		err = r.Get(ctx, request.NamespacedName, v1beta1Instance)
		if err == nil {
			v1Instance = v1beta1Csr2v1Csr(v1beta1Instance)
		}
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	approved, denied := checkCertApprovalCondition(&v1Instance.Status)
	if approved {
		klog.V(4).Infof("csr(%s) is approved", v1Instance.GetName())
		return reconcile.Result{}, nil
	}

	if denied {
		klog.V(4).Infof("csr(%s) is denied", v1Instance.GetName())
		return reconcile.Result{}, nil
	}

	ok, successMsg := isYurtCSR(v1Instance)
	if !ok {
		klog.Infof("csr(%s) is not %s", v1Instance.GetName(), yurtCsr)
		return reconcile.Result{}, nil
	}

	// approve the openyurt related csr
	v1Instance.Status.Conditions = append(v1Instance.Status.Conditions,
		certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateApproved,
			Status:  corev1.ConditionTrue,
			Reason:  "AutoApproved",
			Message: successMsg,
		})

	// Update CertificateSigningRequests
	err = r.updateApproval(ctx, v1Instance)
	if err != nil {
		klog.Errorf("could not approve %s(%s), %v", yurtCsr, v1Instance.GetName(), err)
		return reconcile.Result{}, err
	}
	klog.Infof("successfully approve %s(%s)", yurtCsr, v1Instance.GetName())
	return reconcile.Result{}, nil
}

// updateApproval is used for adding approval info into csr resource
func (r *ReconcileCsrApprover) updateApproval(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (err error) {
	if r.csrV1Supported {
		_, err = r.csrApproverClient.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{})
	} else {
		v1beta1Csr := v1Csr2v1beta1Csr(csr)
		_, err = r.csrApproverClient.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(ctx, v1beta1Csr, metav1.UpdateOptions{})
	}
	return
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

// isYurtTunnelProxyClientCert is used to recognize csr from yurtcoordinator client certificate .
func isYurtCoordinatorClientCert(csr *certificatesv1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if csr.Spec.SignerName != certificatesv1.KubeAPIServerClientSignerName {
		return false
	}

	if len(x509cr.Subject.Organization) != 1 || x509cr.Subject.Organization[0] != yurtcoorrdinatorCert.YurtCoordinatorOrg {
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
