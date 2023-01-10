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
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

func TestIsYurtCSR(t *testing.T) {
	tests := []struct {
		desc string
		csr  *certificatesv1.CertificateSigningRequest
		exp  bool
	}{
		{
			desc: "is not certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type: "PUBLIC KEY",
						}),
				},
			},
			exp: false,
		},
		{
			desc: "can not parse certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: []byte(`MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAlRuRnThUjU8/prwYxbty`),
						}),
				},
			},
			exp: false,
		},
		{
			desc: "is yurthub server related certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
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
			exp: true,
		},
		{
			desc: "is yurthub node client related certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: newCSRData("system:node:xxx", []string{token.YurtHubCSROrg, user.NodesGroup, "openyurt:tenant:xxx"}, []string{}, []net.IP{}),
				},
			},
			exp: true,
		},
		{
			desc: "yurthub node client csr with unknown org",
			csr: &certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: certificatesv1.KubeAPIServerClientSignerName,
					Usages: []certificatesv1.KeyUsage{
						certificatesv1.UsageDigitalSignature,
						certificatesv1.UsageKeyEncipherment,
						certificatesv1.UsageClientAuth,
					},
					Request: newCSRData("system:node:xxx", []string{token.YurtHubCSROrg, user.NodesGroup, "unknown org"}, []string{}, []net.IP{}),
				},
			},
			exp: false,
		},
		{
			desc: "is yurt-tunnel server related tls server certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
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
			exp: true,
		},
		{
			desc: "is yurt-tunnel-server related proxy client certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
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
			exp: true,
		},
		{
			desc: "is yurt-tunnel-agent related certificate request",
			csr: &certificatesv1.CertificateSigningRequest{
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
			exp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act, msg := isYurtCSR(tt.csr)
			if act != tt.exp {
				t.Errorf("the value we want is %v, but the actual value is %v, and the message: %s", tt.exp, act, msg)
			}
		})
	}
}

func TestCheckCertApprovalCondition(t *testing.T) {
	tests := []struct {
		desc   string
		status *certificatesv1.CertificateSigningRequestStatus
		exp    struct {
			approved bool
			denied   bool
		}
	}{
		{
			desc: "approved",
			status: &certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{
						Type: certificatesv1.CertificateApproved,
					},
				},
			},
			exp: struct {
				approved bool
				denied   bool
			}{approved: true, denied: false},
		},
		{
			desc: "denied",
			status: &certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{
						Type: certificatesv1.CertificateDenied,
					},
				},
			},
			exp: struct {
				approved bool
				denied   bool
			}{approved: false, denied: true},
		},
		{
			desc: "approved and denied",
			status: &certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{
						Type: certificatesv1.CertificateApproved,
					},
					{
						Type: certificatesv1.CertificateDenied,
					},
				},
			},
			exp: struct {
				approved bool
				denied   bool
			}{approved: true, denied: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			actApproved, actDenied := checkCertApprovalCondition(tt.status)
			act := struct {
				approved bool
				denied   bool
			}{approved: actApproved, denied: actDenied}
			if act != tt.exp {
				t.Errorf("the value we want is %+v, but the actual value is %+v", tt.exp, act)
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
