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
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"testing"

	certificates "k8s.io/api/certificates/v1beta1"
)

var rsaPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA3xVbRYwmRaFAVUHRm/ynFbOe6pDNEsDIoEE2+7LDrlRndp1w
hzrOd9DPWBcEJIO6ga1U9TdCP1HnOWQLaoM4c4Tngb26ieZYNW2PKhij3wdoN5eO
t7jKupAD1eEChDjsZSN2/OJVLi9/82vAxjmfCzz9icRGlUd2E4Ixtd+EUxz4gCjQ
elNyjEPO28/6TfL3o18jX4UKonk+CKEIotrf1hph0I2/Feb+DeUQIRjvhwzoaauk
2epAUAeunMpatLkwQp6BfDTu/+MkJgcgyHv2qlb+2zYSvvzbVm3lNIa4Mkoe8Bqe
ecLxrp07uxp13SVtJE9EeyVdAIwNg0H7DJ6QDQIDAQABAoIBAA2scHC922avMJNJ
OoDWJqOk49u6zmcU2/c+qBEbbvUThVf25HvVdexQJzVeC8n1LQxfxHJXVb8t1P9m
i3CW5HHoNox0RafIL6XutjS9V+YGvTOTHZNTR1HSG/oTFaVnG85DMzri4Je5H52b
ADDmPUJiFaRJHI5v1+PwOf3M2n6BjcRh9rIwDX/1eSyCWwKc10ZcXWcGTwyXtDz5
lwIyKrUvmuQvjS6Wq84J6dTDQfk2LrXHk92UYaUyrpdQ5lvuOAAfvO8XoOmJvFf+
h1RLnFdk5DB7aWH+DbQvfVEuoTgbhWgRHku/KpdITjTE0ZHp945hINnN/071W5aq
dHo5kx0CgYEA+HE05pV9eAlblq534H586wsCJHKB70Fi3yN2eQdJelwIeBkRwulW
hfLkaXQM/I8BsGgVviBfREq7Zzaz3v5UYirjJOe/X11b9mn7HGcVlNR0ilOKmVrH
SAWr/ZkjIiza25SRYJ/I1y0HE3GMGOdiwj52E4mEMXaExPW762p/AKMCgYEA5d6p
yqguhKzoBRFFA0CXARxu2uTfipfIYIJFAnYVg1fkJocV7mZP8z+qLc1qWUm8Enje
QagAbEZH5SDpEKiHGIGqmODl+qAg9vHXrMOcQabmwGhXK0wn1E3QRoVHQ8N90weB
9A/Mc7LKxDpSwTkWgVMJWxK29U75CXWfWjeAR48CgYAncqI5sqbXdnTqeg1iwfLH
x1mxu9TRzooKcDERioyqNw7JMwHU9wPcBPMro1ekinh0MDKzm6REzbDv9Ime8Lcp
VzH13C5Q0BwYBj/vBJcyqIFQrW8mZnmZ//yNKdGgTYr6rp5ev0A+mlGzTqY2Fhdi
TFSnSYCJ8g2m0HXkLWa5DQKBgQCZxJlQN7Dmj8OloCfKRSq+U4bUZsYir+YaqQoA
230In4K/Qx4om8hfr/bnLMI3eFuW/8Otp/SgeWMeoyVFP3cfrZ2xJsCxJuzmRGFB
8JhWUo+JpkKpdAgwvNzWT9GcQumogR0tZmQeATwih+FT4Bxt5l4bzikVb/6nlUdD
0ly9gQKBgBMBjd83IEJDBrUtWaxtPlt8HDFgTlqgZJxIckW3bnF+7iPTlO7hLpOD
dbALa7x9+2ydqd9lpyd8txi57gYMHuVi1KaBvMDbKAo0SXLNV1Kv73HNO3k/o2+w
k6IJMsIcAOOuF9N1A6awc8mEKiQ53slCbdosjes2Zurzv6gJGLQ+
-----END RSA PRIVATE KEY-----`

func TestIsYurtCSR(t *testing.T) {
	tests := []struct {
		desc string
		csr  *certificates.CertificateSigningRequest
		exp  bool
	}{
		{
			desc: "is not certificate request",
			csr: &certificates.CertificateSigningRequest{
				Spec: certificates.CertificateSigningRequestSpec{
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
			csr: &certificates.CertificateSigningRequest{
				Spec: certificates.CertificateSigningRequestSpec{
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
			desc: "is not a openyurt related certificate request",
			csr: &certificates.CertificateSigningRequest{
				Spec: certificates.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: newCSR([]string{"not_openyurt_organization"}),
						}),
				},
			},
			exp: false,
		},
		{
			desc: "is yurttunnel related certificate request",
			csr: &certificates.CertificateSigningRequest{
				Spec: certificates.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: newCSR([]string{YurttunnelCSROrg}),
						}),
				},
			},
			exp: true,
		},
		{
			desc: "is yurthub related certificate request",
			csr: &certificates.CertificateSigningRequest{
				Spec: certificates.CertificateSigningRequestSpec{
					Request: pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE REQUEST",
							Bytes: newCSR([]string{YurthubCSROrg}),
						}),
				},
			},
			exp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			act := isYurtCSR(tt.csr)
			if act != tt.exp {
				t.Errorf("the value we want is %v, but the actual value is %v", tt.exp, act)
			}
		})
	}
}

func TestCheckCertApprovalCondition(t *testing.T) {
	tests := []struct {
		desc   string
		status *certificates.CertificateSigningRequestStatus
		exp    struct {
			approved bool
			denied   bool
		}
	}{
		{
			desc: "approved",
			status: &certificates.CertificateSigningRequestStatus{
				Conditions: []certificates.CertificateSigningRequestCondition{
					{
						Type: certificates.CertificateApproved,
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
			status: &certificates.CertificateSigningRequestStatus{
				Conditions: []certificates.CertificateSigningRequestCondition{
					{
						Type: certificates.CertificateDenied,
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
			status: &certificates.CertificateSigningRequestStatus{
				Conditions: []certificates.CertificateSigningRequestCondition{
					{
						Type: certificates.CertificateApproved,
					},
					{
						Type: certificates.CertificateDenied,
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

func newCSR(organizations []string) []byte {
	block, _ := pem.Decode([]byte(rsaPrivateKey))
	rsaPriv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse private key: %s", err)
	}

	req := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: organizations,
		},
		DNSNames: []string{
			"openyurt.io",
		},
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, req, rsaPriv)
	if err != nil {
		log.Fatalf("unable to create CSR: %s", err)
	}
	return csr
}
