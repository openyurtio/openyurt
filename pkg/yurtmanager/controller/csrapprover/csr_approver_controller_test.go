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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1beta1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
		"yurt-tunnel-server related proxy client certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-proxy-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-server-proxy-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
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
		"yurt-tunnel-agent related certificate request": {
			obj: &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-agent-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tunnel-agent-client-csr",
					Namespace: "default",
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateSigningRequest",
					APIVersion: "certificates.k8s.io/v1",
				},
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
				if err != nil {
					t.Errorf("expect no error, but got an error, %v", err)
					return
				}

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
