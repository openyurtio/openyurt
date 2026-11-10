/*
Copyright 2024 The OpenYurt Authors.

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

package csrapprover

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"reflect"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

func TestReconcile(t *testing.T) {
	v1beta1SignerName := certificatesv1beta1.KubeAPIServerClientSignerName
	csrData := newCSRData("system:node:xxx", []string{token.YurtHubCSROrg, user.NodesGroup, "unknown org"}, []string{}, []net.IP{})
	testcases := map[string]struct {
		obj            runtime.Object
		csrV1Supported bool
		skipRequest    bool
		expectedObj    runtime.Object
	}{
		"yurthub server related certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Request: newCSRData("system:node:xxx", []string{user.NodesGroup}, []string{}, []net.IP{net.ParseIP("127.0.0.1")}),
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "AutoApproved",
							Message: "Auto approving openyurt tls server certificate",
						},
					},
				},
			},
		},
		"yurthub node client related certificate request": {
			obj: &certificatesv1beta1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1beta1.CertificateSigningRequestSpec{
					SignerName: &v1beta1SignerName,
					Usages: []certificatesv1beta1.KeyUsage{
						certificatesv1beta1.UsageDigitalSignature,
						certificatesv1beta1.UsageKeyEncipherment,
						certificatesv1beta1.UsageClientAuth,
					},
					Request: newCSRData("system:node:xxx", []string{token.YurtHubCSROrg, user.NodesGroup, "openyurt:tenant:xxx"}, []string{}, []net.IP{}),
				},
				Status: certificatesv1beta1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1beta1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1beta1.RequestConditionType("test"),
							Status:  corev1.ConditionTrue,
							Reason:  "test",
							Message: "test",
						},
					},
				},
			},
			csrV1Supported: false,
			skipRequest:    true,
			expectedObj: &certificatesv1beta1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1beta1.CertificateSigningRequestSpec{
					SignerName: &v1beta1SignerName,
					Usages: []certificatesv1beta1.KeyUsage{
						certificatesv1beta1.UsageDigitalSignature,
						certificatesv1beta1.UsageKeyEncipherment,
						certificatesv1beta1.UsageClientAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1beta1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1beta1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1beta1.RequestConditionType("test"),
							Status:  corev1.ConditionTrue,
							Reason:  "test",
							Message: "test",
						},
						{
							Type:    certificatesv1beta1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "AutoApproved",
							Message: "Auto approving yurthub node client certificate",
						},
					},
				},
			},
		},
		"yurt-tunnel server related tls server certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Request: newCSRData("system:node:xxx", []string{user.NodesGroup}, []string{"x-tunnel-server-svc"}, []net.IP{net.ParseIP("127.0.0.1")}),
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "AutoApproved",
							Message: "Auto approving openyurt tls server certificate",
						},
					},
				},
			},
		},
		"raven proxy user certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-proxy-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username:   "system:serviceaccount:kube-system:raven-agent-account",
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: newCSRData(constants.YurtTunnelProxyClientCSRCN, []string{constants.YurtTunnelCSROrg}, []string{}, []net.IP{}),
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-proxy-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username:   "system:serviceaccount:kube-system:raven-agent-account",
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "AutoApproved",
							Message: "Auto approving tunnel-server proxy client certificate",
						},
					},
				},
			},
		},
		"raven proxy client certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-agent-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username:   "system:serviceaccount:kube-system:raven-agent-account",
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: newCSRData(constants.YurtTunnelAgentCSRCN, []string{constants.YurtTunnelCSROrg}, []string{"node-name-xxx"}, []net.IP{net.ParseIP("127.0.0.1")}),
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-agent-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username:   "system:serviceaccount:kube-system:raven-agent-account",
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "AutoApproved",
							Message: "Auto approving tunnel-agent client certificate",
						},
					},
				},
			},
		},
		"it is not a certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type: "PUBLIC KEY",
						}),
				},
			},
			csrV1Supported: true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type: "PUBLIC KEY",
						}),
				},
			},
		},
		"can not parse certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: []byte(`MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAlRuRnThUjU8/prwYxbty`),
						}),
				},
			},
			csrV1Supported: true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: []byte(`MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAlRuRnThUjU8/prwYxbty`),
						}),
				},
			},
		},
		"yurthub node client csr with unknown org": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: csrData,
				},
			},
			csrV1Supported: true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: csrData,
				},
				Status: certificatesv1.CertificateSigningRequestStatus{},
			},
		},
		"csr has already been approved": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Request: newCSRData("system:node:xxx", []string{user.NodesGroup}, []string{}, []net.IP{net.ParseIP("127.0.0.1")}),
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "init approved",
							Message: "csr has already been approved",
						},
					},
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateApproved,
							Status:  corev1.ConditionTrue,
							Reason:  "init approved",
							Message: "csr has already been approved",
						},
					},
				},
			},
		},
		"csr has already been denied": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					Request: newCSRData("system:node:xxx", []string{user.NodesGroup}, []string{}, []net.IP{net.ParseIP("127.0.0.1")}),
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateDenied,
							Status:  corev1.ConditionTrue,
							Reason:  "init denied",
							Message: "csr has already been denied",
						},
					},
				},
			},
			csrV1Supported: true,
			skipRequest:    true,
			expectedObj: &certificatesv1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "server-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeletServingSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageServerAuth,
					},
					// don't compare CSR data
					Request: []byte{},
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{
							Type:    certificatesv1.CertificateDenied,
							Status:  corev1.ConditionTrue,
							Reason:  "init denied",
							Message: "csr has already been denied",
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			obj := tc.obj.DeepCopyObject()
			accessor := meta.NewAccessor()
			name, _ := accessor.Name(obj)
			ns, _ := accessor.Namespace(obj)
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      name,
					Namespace: ns,
				},
			}
			c := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.obj).WithStatusSubresource([]client.Object{&certificatesv1beta1.CertificateSigningRequest{}}...).Build()
			csrApproverController := &ReconcileCsrApprover{
				Client:         c,
				csrV1Supported: tc.csrV1Supported,
			}

			_, err := csrApproverController.Reconcile(context.Background(), req)
			if err != nil {
				t.Errorf("expect an error, but got no error")
			} else {

				if tc.csrV1Supported {
					v1Instance := &certificatesv1.CertificateSigningRequest{}
					err = csrApproverController.Get(context.Background(), req.NamespacedName, v1Instance)
					if err != nil {
						t.Errorf("couldn't get csr v1 instance, %v", err)
						return
					}
					// clear resource version and csr data
					accessor.SetResourceVersion(v1Instance, "")
					if tc.skipRequest {
						v1Instance.Spec.Request = []byte{}
					}
					if !reflect.DeepEqual(v1Instance, tc.expectedObj) {
						t.Errorf("expect object %#+v\n, but got %#+v\n", tc.expectedObj, v1Instance)
					}
				} else {
					v1beta1Instance := &certificatesv1beta1.CertificateSigningRequest{}
					err = csrApproverController.Get(context.Background(), req.NamespacedName, v1beta1Instance)
					if err != nil {
						t.Errorf("couldn't get csr, %v", err)
						return
					}
					// clear resource version and csr data
					accessor.SetResourceVersion(v1beta1Instance, "")
					if tc.skipRequest {
						v1beta1Instance.Spec.Request = []byte{}
					}
					if !reflect.DeepEqual(v1beta1Instance, tc.expectedObj) {
						t.Errorf("expect object %#+v\n, but got %#+v\n", tc.expectedObj, v1beta1Instance)
					}
				}
			}
		})
	}
}

func TestReconcileRavenCSRRequester(t *testing.T) {
	requesters := []struct {
		name         string
		username     string
		wantApproved bool
	}{
		{"raven serviceaccount", "system:serviceaccount:kube-system:raven-agent-account", true},
		{"wrong namespace", "system:serviceaccount:default:raven-agent-account", false},
		{"wrong serviceaccount", "system:serviceaccount:kube-system:other-agent", false},
		{"username suffix", "system:serviceaccount:kube-system:raven-agent-account-extra", false},
		{"missing identity", "", false},
		{"certificate subject is not requester", "tunnel-agent-client", false},
		{"legacy tunnel agent", "system:serviceaccount:kube-system:yurt-tunnel-agent", false},
		{"legacy tunnel server", "system:serviceaccount:kube-system:yurt-tunnel-server", false},
		{"admin has no exemption", "kubernetes-admin", false},
	}
	for _, commonName := range []string{
		"tunnel-agent-client", "tunnel-proxy-client", "system:node:tunnel-proxy-server",
	} {
		t.Run(commonName, func(t *testing.T) {
			csr := newRavenCSR(t, commonName)
			for _, requester := range requesters {
				t.Run(requester.name, func(t *testing.T) {
					request := csr.DeepCopy()
					request.Spec.Username = requester.username
					// Groups and other requester metadata must not substitute for username.
					request.Spec.UID = "test-serviceaccount-uid"
					request.Spec.Groups = []string{"system:serviceaccounts:kube-system", "system:authenticated"}
					request.Spec.Extra = map[string]certificatesv1.ExtraValue{
						"authentication.kubernetes.io/pod-name": {"raven-agent-test"},
					}
					for _, v1Supported := range []bool{true, false} {
						t.Run(fmt.Sprintf("v1Supported=%t", v1Supported), func(t *testing.T) {
							assertCSRReconcile(t, request, v1Supported, requester.wantApproved)
						})
					}
				})
			}
		})
	}
}

func TestRavenCSRTemplates(t *testing.T) {
	testcases := []struct {
		name         string
		mutate       func(*certificatesv1.CertificateSigningRequest, *x509.CertificateRequest)
		wantApproved bool
	}{
		{"valid template", func(_ *certificatesv1.CertificateSigningRequest, _ *x509.CertificateRequest) {}, true},
		{"DNS SAN only", func(_ *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			req.IPAddresses = nil
		}, true},
		{"IP SAN only", func(_ *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			req.DNSNames = nil
		}, true},
		{"wrong signer", func(csr *certificatesv1.CertificateSigningRequest, _ *x509.CertificateRequest) {
			csr.Spec.SignerName = certificatesv1.KubeAPIServerClientKubeletSignerName
		}, false},
		{"missing organization", func(_ *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			req.Subject.Organization = nil
		}, false},
		{"wrong organization", func(_ *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			req.Subject.Organization = []string{"system:masters"}
		}, false},
		{"multiple organizations", func(_ *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			req.Subject.Organization = append(req.Subject.Organization, "system:masters")
		}, false},
		{"missing usages", func(csr *certificatesv1.CertificateSigningRequest, _ *x509.CertificateRequest) {
			csr.Spec.Usages = nil
		}, false},
		{"missing key encipherment", func(csr *certificatesv1.CertificateSigningRequest, _ *x509.CertificateRequest) {
			csr.Spec.Usages = []certificatesv1.KeyUsage{csr.Spec.Usages[0], csr.Spec.Usages[2]}
		}, false},
		{"extra usage", func(csr *certificatesv1.CertificateSigningRequest, _ *x509.CertificateRequest) {
			csr.Spec.Usages = append(csr.Spec.Usages, certificatesv1.UsageCodeSigning)
		}, false},
		{"coordinator recognizer bypass", func(csr *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			csr.Spec.SignerName = certificatesv1.KubeAPIServerClientSignerName
			csr.Spec.Usages = []certificatesv1.KeyUsage{
				certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, certificatesv1.UsageClientAuth,
			}
			req.Subject.Organization = []string{"openyurt:yurt-coordinator"}
		}, false},
		{"yurthub recognizer bypass", func(csr *certificatesv1.CertificateSigningRequest, req *x509.CertificateRequest) {
			csr.Spec.SignerName = certificatesv1.KubeAPIServerClientSignerName
			csr.Spec.Usages = []certificatesv1.KeyUsage{
				certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, certificatesv1.UsageClientAuth,
			}
			req.Subject.Organization = []string{token.YurtHubCSROrg, user.NodesGroup}
		}, false},
	}
	for _, commonName := range []string{
		"tunnel-agent-client", "tunnel-proxy-client", "system:node:tunnel-proxy-server",
	} {
		t.Run(commonName, func(t *testing.T) {
			csr := newRavenCSR(t, commonName)
			for _, tc := range testcases {
				t.Run(tc.name, func(t *testing.T) {
					request := csr.DeepCopy()
					block, _ := pem.Decode(request.Spec.Request)
					if block == nil {
						t.Fatal("invalid test CSR PEM")
					}
					x509cr, err := x509.ParseCertificateRequest(block.Bytes)
					if err != nil {
						t.Fatal(err)
					}
					tc.mutate(request, x509cr)
					request.Spec.Request = newCSRData(commonName, x509cr.Subject.Organization, x509cr.DNSNames, x509cr.IPAddresses)
					for _, v1Supported := range []bool{true, false} {
						t.Run(fmt.Sprintf("v1Supported=%t", v1Supported), func(t *testing.T) {
							assertCSRReconcile(t, request, v1Supported, tc.wantApproved)
						})
					}
				})
			}
			if commonName == "system:node:tunnel-proxy-server" {
				t.Run("server requires SAN", func(t *testing.T) {
					request := csr.DeepCopy()
					request.Spec.Request = newCSRData(commonName, []string{user.NodesGroup}, nil, nil)
					for _, v1Supported := range []bool{true, false} {
						assertCSRReconcile(t, request, v1Supported, false)
					}
				})
			}
		})
	}
}

func TestReconcileNonRavenCSR(t *testing.T) {
	testcases := []struct {
		name         string
		commonName   string
		organization []string
		signerName   string
		username     string
		authUsage    certificatesv1.KeyUsage
		wantApproved bool
	}{
		{
			name: "yurthub node client", commonName: "system:node:worker",
			organization: []string{token.YurtHubCSROrg, user.NodesGroup},
			signerName:   certificatesv1.KubeAPIServerClientSignerName, username: "system:node:worker",
			authUsage: certificatesv1.UsageClientAuth, wantApproved: true,
		},
		{
			name: "other TLS server", commonName: "system:node:worker", organization: []string{user.NodesGroup},
			signerName: certificatesv1.KubeletServingSignerName, username: "system:node:worker",
			authUsage: certificatesv1.UsageServerAuth, wantApproved: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			csr := &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "non-raven-csr"},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Username: tc.username, SignerName: tc.signerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, tc.authUsage,
					},
					Request: newCSRData(tc.commonName, tc.organization, []string{"worker"}, nil),
				},
			}
			for _, v1Supported := range []bool{true, false} {
				t.Run(fmt.Sprintf("v1Supported=%t", v1Supported), func(t *testing.T) {
					assertCSRReconcile(t, csr, v1Supported, tc.wantApproved)
				})
			}
		})
	}
}

func newRavenCSR(t *testing.T, commonName string) *certificatesv1.CertificateSigningRequest {
	t.Helper()
	signerName := certificatesv1.KubeAPIServerClientSignerName
	organization := constants.YurtTunnelCSROrg
	authUsage := certificatesv1.UsageClientAuth
	if commonName == "system:node:tunnel-proxy-server" {
		signerName = certificatesv1.KubeletServingSignerName
		organization = user.NodesGroup
		authUsage = certificatesv1.UsageServerAuth
	}
	request := newCSRData(commonName, []string{organization}, []string{"raven-test"}, []net.IP{net.ParseIP("127.0.0.1")})
	if len(request) == 0 {
		t.Fatal("failed to generate test CSR")
	}
	return &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "raven-csr"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username:   "system:serviceaccount:kube-system:raven-agent-account",
			SignerName: signerName,
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageDigitalSignature, certificatesv1.UsageKeyEncipherment, authUsage,
			},
			Request: request,
		},
	}
}

func assertCSRReconcile(t *testing.T, csr *certificatesv1.CertificateSigningRequest, v1Supported, wantApproved bool) {
	t.Helper()
	var obj client.Object = csr.DeepCopy()
	if !v1Supported {
		// Build the beta request independently so a broken production conversion cannot mask the requester.
		beta := &certificatesv1beta1.CertificateSigningRequest{
			ObjectMeta: csr.ObjectMeta,
			Spec: certificatesv1beta1.CertificateSigningRequestSpec{
				Request:    csr.Spec.Request,
				SignerName: &csr.Spec.SignerName,
				Username:   csr.Spec.Username,
				UID:        csr.Spec.UID,
				Groups:     csr.Spec.Groups,
			},
		}
		for _, usage := range csr.Spec.Usages {
			beta.Spec.Usages = append(beta.Spec.Usages, certificatesv1beta1.KeyUsage(usage))
		}
		if csr.Spec.Extra != nil {
			beta.Spec.Extra = make(map[string]certificatesv1beta1.ExtraValue, len(csr.Spec.Extra))
			for key, value := range csr.Spec.Extra {
				beta.Spec.Extra[key] = certificatesv1beta1.ExtraValue(value)
			}
		}
		obj = beta
	}
	approvalUpdates := 0
	c := fakeclient.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(obj).
		WithStatusSubresource(&certificatesv1.CertificateSigningRequest{}, &certificatesv1beta1.CertificateSigningRequest{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if subResource != "approval" {
					t.Fatalf("unexpected subresource update: %s", subResource)
				}
				approvalUpdates++
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}).Build()
	r := &ReconcileCsrApprover{Client: c, csrV1Supported: v1Supported}
	key := client.ObjectKeyFromObject(obj)
	result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile returned an error: %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Errorf("unexpected reconcile result: %+v", result)
	}
	actual := obj.DeepCopyObject().(client.Object)
	if err := c.Get(context.Background(), key, actual); err != nil {
		t.Fatal(err)
	}
	if !wantApproved {
		if approvalUpdates != 0 {
			t.Errorf("unauthorized or invalid CSR received %d approval updates", approvalUpdates)
		}
		actual.SetResourceVersion(obj.GetResourceVersion())
		if !reflect.DeepEqual(actual, obj) {
			t.Error("CSR must remain unchanged and Pending")
		}
		return
	}
	if approvalUpdates != 1 {
		t.Errorf("expected one approval update, got %d", approvalUpdates)
	}
	switch actual := actual.(type) {
	case *certificatesv1.CertificateSigningRequest:
		if !reflect.DeepEqual(actual.Spec, csr.Spec) {
			t.Errorf("approval changed CSR spec: %+v", actual.Spec)
		}
		if len(actual.Status.Conditions) != 1 || actual.Status.Conditions[0].Type != certificatesv1.CertificateApproved ||
			actual.Status.Conditions[0].Status != corev1.ConditionTrue || actual.Status.Conditions[0].Reason != "AutoApproved" {
			t.Errorf("expected Approved condition, got %+v", actual.Status.Conditions)
		}
	case *certificatesv1beta1.CertificateSigningRequest:
		if !reflect.DeepEqual(actual.Spec, obj.(*certificatesv1beta1.CertificateSigningRequest).Spec) {
			t.Errorf("approval changed beta CSR spec: %+v", actual.Spec)
		}
		if len(actual.Status.Conditions) != 1 || actual.Status.Conditions[0].Type != certificatesv1beta1.CertificateApproved ||
			actual.Status.Conditions[0].Status != corev1.ConditionTrue || actual.Status.Conditions[0].Reason != "AutoApproved" {
			t.Errorf("expected Approved condition, got %+v", actual.Status.Conditions)
		}
	}
}

func newCSRData(commonName string, organizations, dnsNames []string, ipAddresses []net.IP) []byte {
	// Generate a new private key.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		klog.Errorf("failed to create private key, %v", err)
		return []byte{}
	}

	req := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: organizations,
		},
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}

	csr, err := cert.MakeCSRFromTemplate(privateKey, req)
	if err != nil {
		klog.Errorf("failed to make csr, %v", err)
		return []byte{}
	}
	return csr
}

func TestCheckCertApprovalCondition(t *testing.T) {
	// Empty conditions
	approved, denied := checkCertApprovalCondition(&certificatesv1.CertificateSigningRequestStatus{})
	if approved || denied {
		t.Error("empty conditions should be neither approved nor denied")
	}

	// Approved
	approved, denied = checkCertApprovalCondition(&certificatesv1.CertificateSigningRequestStatus{
		Conditions: []certificatesv1.CertificateSigningRequestCondition{
			{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue},
		},
	})
	if !approved || denied {
		t.Error("should be approved and not denied")
	}

	// Denied
	approved, denied = checkCertApprovalCondition(&certificatesv1.CertificateSigningRequestStatus{
		Conditions: []certificatesv1.CertificateSigningRequestCondition{
			{Type: certificatesv1.CertificateDenied, Status: corev1.ConditionTrue},
		},
	})
	if approved || !denied {
		t.Error("should be denied and not approved")
	}

	// Both (edge case)
	approved, denied = checkCertApprovalCondition(&certificatesv1.CertificateSigningRequestStatus{
		Conditions: []certificatesv1.CertificateSigningRequestCondition{
			{Type: certificatesv1.CertificateApproved},
			{Type: certificatesv1.CertificateDenied},
		},
	})
	if !approved || !denied {
		t.Error("should be both approved and denied when both conditions present")
	}
}

func TestV1Csr2V1beta1Csr(t *testing.T) {
	signerName := "kubernetes.io/kube-apiserver-client"
	v1csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "test-csr"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    []byte("test-request"),
			SignerName: signerName,
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature},
			Username:   "system:serviceaccount:kube-system:raven-agent-account",
			UID:        "raven-serviceaccount-uid",
			Groups:     []string{"system:serviceaccounts", "system:authenticated"},
			Extra: map[string]certificatesv1.ExtraValue{
				"authentication.kubernetes.io/pod-name": {"raven-agent-test"},
			},
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{Type: certificatesv1.CertificateApproved, Reason: "approved", Message: "ok"},
			},
		},
	}

	v1beta1Csr := v1Csr2v1beta1Csr(v1csr)
	if v1beta1Csr.Name != "test-csr" {
		t.Errorf("expected name test-csr, got %s", v1beta1Csr.Name)
	}
	if *v1beta1Csr.Spec.SignerName != signerName {
		t.Errorf("signer name mismatch")
	}
	if len(v1beta1Csr.Spec.Usages) != 1 {
		t.Errorf("expected 1 usage, got %d", len(v1beta1Csr.Spec.Usages))
	}
	if len(v1beta1Csr.Status.Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(v1beta1Csr.Status.Conditions))
	}
	if v1beta1Csr.Spec.Username != v1csr.Spec.Username || v1beta1Csr.Spec.UID != v1csr.Spec.UID {
		t.Errorf("requester identity was lost: username=%q, uid=%q", v1beta1Csr.Spec.Username, v1beta1Csr.Spec.UID)
	}
	if !reflect.DeepEqual(v1beta1Csr.Spec.Groups, v1csr.Spec.Groups) {
		t.Errorf("requester groups were lost: %v", v1beta1Csr.Spec.Groups)
	}
	expectedExtra := map[string]certificatesv1beta1.ExtraValue{
		"authentication.kubernetes.io/pod-name": {"raven-agent-test"},
	}
	if !reflect.DeepEqual(v1beta1Csr.Spec.Extra, expectedExtra) {
		t.Errorf("requester extra was lost: %v", v1beta1Csr.Spec.Extra)
	}
}

func TestV1beta1Csr2V1Csr(t *testing.T) {
	// nil input
	if v1beta1Csr2v1Csr(nil) != nil {
		t.Error("nil input should return nil")
	}

	signerName := "kubernetes.io/kube-apiserver-client"
	v1beta1csr := &certificatesv1beta1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "test-csr"},
		Spec: certificatesv1beta1.CertificateSigningRequestSpec{
			Request:    []byte("test-request"),
			SignerName: &signerName,
			Usages:     []certificatesv1beta1.KeyUsage{certificatesv1beta1.UsageDigitalSignature},
			Username:   "system:serviceaccount:kube-system:raven-agent-account",
			UID:        "raven-serviceaccount-uid",
			Groups:     []string{"system:serviceaccounts", "system:authenticated"},
			Extra: map[string]certificatesv1beta1.ExtraValue{
				"authentication.kubernetes.io/pod-name": {"raven-agent-test"},
			},
		},
	}
	v1csr := v1beta1Csr2v1Csr(v1beta1csr)
	if v1csr.Name != "test-csr" {
		t.Errorf("expected name test-csr, got %s", v1csr.Name)
	}
	if v1csr.Spec.SignerName != signerName {
		t.Errorf("signer name mismatch")
	}
	if v1csr.Spec.Username != v1beta1csr.Spec.Username || v1csr.Spec.UID != v1beta1csr.Spec.UID {
		t.Errorf("requester identity was lost: username=%q, uid=%q", v1csr.Spec.Username, v1csr.Spec.UID)
	}
	if !reflect.DeepEqual(v1csr.Spec.Groups, v1beta1csr.Spec.Groups) {
		t.Errorf("requester groups were lost: %v", v1csr.Spec.Groups)
	}
	expectedExtra := map[string]certificatesv1.ExtraValue{
		"authentication.kubernetes.io/pod-name": {"raven-agent-test"},
	}
	if !reflect.DeepEqual(v1csr.Spec.Extra, expectedExtra) {
		t.Errorf("requester extra was lost: %v", v1csr.Spec.Extra)
	}

	// nil signer name
	v1beta1csrNoSigner := &certificatesv1beta1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	v1csrNoSigner := v1beta1Csr2v1Csr(v1beta1csrNoSigner)
	if v1csrNoSigner.Spec.SignerName != "" {
		t.Error("expected empty signer name")
	}
}

func TestUsagesToSet(t *testing.T) {
	usages := []certificatesv1.KeyUsage{
		certificatesv1.UsageDigitalSignature,
		certificatesv1.UsageKeyEncipherment,
	}
	result := usagesToSet(usages)
	if !result.Has(string(certificatesv1.UsageDigitalSignature)) {
		t.Error("missing UsageDigitalSignature")
	}
	if !result.Has(string(certificatesv1.UsageKeyEncipherment)) {
		t.Error("missing UsageKeyEncipherment")
	}

	// Empty
	emptySet := usagesToSet(nil)
	if emptySet.Len() != 0 {
		t.Error("expected empty set for nil input")
	}
}

func TestIsYurtCSRInvalidInput(t *testing.T) {
	// CSR with no request data
	csr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: []byte("not a PEM"),
		},
	}
	ok, msg := isYurtCSR(csr)
	if ok {
		t.Error("expected false for invalid PEM")
	}
	if msg != "" {
		t.Errorf("expected empty message, got %s", msg)
	}
}
