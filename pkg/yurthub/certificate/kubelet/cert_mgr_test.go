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

package kubelet

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
)

var storeCertData = newCertificateData(`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJALMb7ecMIk3MMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwIBcNMTcwNDI2MjMyNjUyWhgPMjExNzA0
MDIyMzI2NTJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAtBMa7NWpv3BVlKTCPGO/LEsguKqWHBtKzweMY2CV
tAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5MzP2H5QIDAQABo1AwTjAdBgNV
HQ4EFgQU22iy8aWkNSxv0nBxFxerfsvnZVMwHwYDVR0jBBgwFoAU22iy8aWkNSxv
0nBxFxerfsvnZVMwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAEOefGbV
NcHxklaW06w6OBYJPwpIhCVozC1qdxGX1dg8VkEKzjOzjgqVD30m59OFmSlBmHsl
nkVA6wyOSDYBf3o=
-----END CERTIFICATE-----`, `-----BEGIN RSA PRIVATE KEY-----
MIIBUwIBADANBgkqhkiG9w0BAQEFAASCAT0wggE5AgEAAkEAtBMa7NWpv3BVlKTC
PGO/LEsguKqWHBtKzweMY2CVtAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5M
zP2H5QIDAQABAkAS9BfXab3OKpK3bIgNNyp+DQJKrZnTJ4Q+OjsqkpXvNltPJosf
G8GsiKu/vAt4HGqI3eU77NvRI+mL4MnHRmXBAiEA3qM4FAtKSRBbcJzPxxLEUSwg
XSCcosCktbkXvpYrS30CIQDPDxgqlwDEJQ0uKuHkZI38/SPWWqfUmkecwlbpXABK
iQIgZX08DA8VfvcA5/Xj1Zjdey9FVY6POLXen6RPiabE97UCICp6eUW7ht+2jjar
e35EltCRCjoejRHTuN9TC0uCoVipAiAXaJIx/Q47vGwiw6Y8KXsNU6y54gTbOSxX
54LzHNk/+Q==
-----END RSA PRIVATE KEY-----`)

type certificateData struct {
	keyPEM         []byte
	certificatePEM []byte
	certificate    *tls.Certificate
}

func newCertificateData(certificatePEM string, keyPEM string) *certificateData {
	certificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize certificate: %v", err))
	}
	certs, err := x509.ParseCertificates(certificate.Certificate[0])
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize certificate leaf: %v", err))
	}
	certificate.Leaf = certs[0]
	return &certificateData{
		keyPEM:         []byte(keyPEM),
		certificatePEM: []byte(certificatePEM),
		certificate:    &certificate,
	}
}

func TestLoadFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "k8s-test-load-cert-key-blocks")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()

	pairFile := filepath.Join(dir, "kubelet-pair.pem")

	tests := []struct {
		desc string
		data []byte
	}{
		{desc: "cert and key", data: bytes.Join([][]byte{storeCertData.certificatePEM, storeCertData.keyPEM}, []byte("\n"))},
		{desc: "key and cert", data: bytes.Join([][]byte{storeCertData.keyPEM, storeCertData.certificatePEM}, []byte("\n"))},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if err := ioutil.WriteFile(pairFile, tt.data, 0600); err != nil {
				t.Fatalf("Unable to create the file %q: %v", pairFile, err)
			}
			cert, err := loadFile(pairFile)
			if err != nil {
				t.Fatalf("Could not load certificate from disk: %v", err)
			}
			if cert == nil {
				t.Fatalf("There was no error, but no certificate data was returned.")
			}
			if cert.Leaf == nil {
				t.Fatalf("Got an empty leaf, expected private data.")
			}
		})
	}
}

func TestCurrent(t *testing.T) {
	dir, err := ioutil.TempDir("", "k8s-test-load-cert-key-blocks")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()

	pairFile := filepath.Join(dir, "kubelet-client-current.pem")
	certData := bytes.Join([][]byte{storeCertData.certificatePEM, storeCertData.keyPEM}, []byte("\n"))

	if err := ioutil.WriteFile(pairFile, certData, 0600); err != nil {
		t.Fatalf("Unable to create the file %q: %v", pairFile, err)
	}

	u, _ := url.Parse("http://127.0.0.1:8080")
	cfg := &config.YurtHubConfiguration{
		RemoteServers: []*url.URL{u},
	}
	// new kubelet cert manager
	m, err := NewKubeletCertManager(cfg, 10*time.Second, dir)
	if err != nil {
		t.Errorf("failed to new kubelet cert manager, %v", err)
	}
	m.Start()
	defer m.Stop()

	// wait over 10s for cert update
	time.Sleep(15 * time.Second)

	// verify current cert
	cert := m.Current()
	if cert == nil {
		t.Fatalf("no certificate data was returned")
	}

	if cert.Leaf == nil {
		t.Errorf("Got an empty leaf, expected private data")
	}
}
