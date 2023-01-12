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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"reflect"
	"testing"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/client-go/util/certificate"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

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
var bootstrapCertData = newCertificateData(
	`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJANXr+UzRFq4TMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwIBcNMTcwNDI2MjMyNzMyWhgPMjExNzA0
MDIyMzI3MzJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAqvbkN4RShH1rL37JFp4fZPnn0JUhVWWsrP8NOomJ
pXdBDUMGWuEQIsZ1Gf9JrCQLu6ooRyHSKRFpAVbMQ3ABJwIDAQABo1AwTjAdBgNV
HQ4EFgQUEGBc6YYheEZ/5MhwqSUYYPYRj2MwHwYDVR0jBBgwFoAUEGBc6YYheEZ/
5MhwqSUYYPYRj2MwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAIyNmznk
5dgJY52FppEEcfQRdS5k4XFPc22SHPcz77AHf5oWZ1WG9VezOZZPp8NCiFDDlDL8
yma33a5eMyTjLD8=
-----END CERTIFICATE-----`, `-----BEGIN RSA PRIVATE KEY-----
MIIBVAIBADANBgkqhkiG9w0BAQEFAASCAT4wggE6AgEAAkEAqvbkN4RShH1rL37J
Fp4fZPnn0JUhVWWsrP8NOomJpXdBDUMGWuEQIsZ1Gf9JrCQLu6ooRyHSKRFpAVbM
Q3ABJwIDAQABAkBC2OBpGLMPHN8BJijIUDFkURakBvuOoX+/8MYiYk7QxEmfLCk6
L6r+GLNFMfXwXcBmXtMKfZKAIKutKf098JaBAiEA10azfqt3G/5owrNA00plSyT6
ZmHPzY9Uq1p/QTR/uOcCIQDLTkfBkLHm0UKeobbO/fSm6ZflhyBRDINy4FvwmZMt
wQIgYV/tmQJeIh91q3wBepFQOClFykG8CTMoDUol/YyNqUkCIHfp6Rr7fGL3JIMq
QQgf9DCK8SPZqq8DYXjdan0kKBJBAiEAyDb+07o2gpggo8BYUKSaiRCiyXfaq87f
eVqgpBq/QN4=
-----END RSA PRIVATE KEY-----`)
var apiServerCertData = newCertificateData(
	`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJAIydTIADd+yqMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTIwIBcNMTcwNDI2MjMyNDU4WhgPMjExNzA0
MDIyMzI0NThaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTIwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAuiRet28DV68Dk4A8eqCaqgXmymamUEjW/DxvIQqH
3lbhtm8BwSnS9wUAajSLSWiq3fci2RbRgaSPjUrnbOHCLQIDAQABo1AwTjAdBgNV
HQ4EFgQU0vhI4OPGEOqT+VAWwxdhVvcmgdIwHwYDVR0jBBgwFoAU0vhI4OPGEOqT
+VAWwxdhVvcmgdIwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBALNeJGDe
nV5cXbp9W1bC12Tc8nnNXn4ypLE2JTQAvyp51zoZ8hQoSnRVx/VCY55Yu+br8gQZ
+tW+O/PoE7B3tuY=
-----END CERTIFICATE-----`, `-----BEGIN RSA PRIVATE KEY-----
MIIBVgIBADANBgkqhkiG9w0BAQEFAASCAUAwggE8AgEAAkEAuiRet28DV68Dk4A8
eqCaqgXmymamUEjW/DxvIQqH3lbhtm8BwSnS9wUAajSLSWiq3fci2RbRgaSPjUrn
bOHCLQIDAQABAkEArDR1g9IqD3aUImNikDgAngbzqpAokOGyMoxeavzpEaFOgCzi
gi7HF7yHRmZkUt8CzdEvnHSqRjFuaaB0gGA+AQIhAOc8Z1h8ElLRSqaZGgI3jCTp
Izx9HNY//U5NGrXD2+ttAiEAzhOqkqI4+nDab7FpiD7MXI6fO549mEXeVBPvPtsS
OcECIQCIfkpOm+ZBBpO3JXaJynoqK4gGI6ALA/ik6LSUiIlfPQIhAISjd9hlfZME
bDQT1r8Q3Gx+h9LRqQeHgPBQ3F5ylqqBAiBaJ0hkYvrIdWxNlcLqD3065bJpHQ4S
WQkuZUQN1M/Xvg==
-----END RSA PRIVATE KEY-----`)

type fakeStore struct {
	cert *tls.Certificate
}

func (s *fakeStore) Current() (*tls.Certificate, error) {
	if s.cert == nil {
		noKeyErr := certificate.NoCertKeyError("")
		return nil, &noKeyErr
	}
	return s.cert, nil
}

// Accepts the PEM data for the cert/key pair and makes the new cert/key
// pair the 'current' pair, that will be returned by future calls to
// Current().
func (s *fakeStore) Update(certPEM, keyPEM []byte) (*tls.Certificate, error) {
	// In order to make the mocking work, whenever a cert/key pair is passed in
	// to be updated in the mock store, assume that the certificate manager
	// generated the key, and then asked the mock CertificateSigningRequest API
	// to sign it, then the faked API returned a canned response. The canned
	// signing response will not match the generated key. In order to make
	// things work out, search here for the correct matching key and use that
	// instead of the passed in key. That way this file of test code doesn't
	// have to implement an actual certificate signing process.
	for _, tc := range []*certificateData{storeCertData, bootstrapCertData, apiServerCertData} {
		if bytes.Equal(tc.certificatePEM, certPEM) {
			keyPEM = tc.keyPEM
		}
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.cert = &cert
	s.cert.Leaf = &x509.Certificate{
		NotBefore: now.Add(-24 * time.Hour),
		NotAfter:  now.Add(24 * time.Hour),
	}
	return s.cert, nil
}

func TestGenTLSConfigUseCurrentCertAndCertPool(t *testing.T) {
	store := &fakeStore{
		cert: storeCertData.certificate,
	}
	cm, err := certificate.NewManager(&certificate.Config{
		Template:         &x509.CertificateRequest{},
		Usages:           []certificatesv1.KeyUsage{},
		CertificateStore: store,
	})
	if err != nil {
		t.Fatalf("Failed to initialize the certificate manager: %v", err)
	}

	tests := []struct {
		name   string
		m      certificate.Manager
		root   *x509.CertPool
		mode   string
		expect error
	}{
		{
			"server mode",
			cm,
			&x509.CertPool{},
			"server",
			nil,
		},
		{
			"client mode",
			cm,
			&x509.CertPool{},
			"client",
			nil,
		},
		{
			"default mode",
			cm,
			&x509.CertPool{},
			"ran",
			fmt.Errorf("unsupported cert manager mode(only server or client), ran"),
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := GenTLSConfigUseCurrentCertAndCertPool(tt.m.Current, tt.root, tt.mode)

				if !reflect.DeepEqual(get, tt.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestGenRootCertPool(t *testing.T) {
	tests := []struct {
		name       string
		kubeConfig string
		caFile     string
		expect     *x509.CertPool
	}{
		{
			"empty kubeconfig caFile",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := GenRootCertPool(tt.kubeConfig, tt.caFile)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestGenTLSConfigUseCertMgrAndCA(t *testing.T) {
	store := &fakeStore{
		cert: storeCertData.certificate,
	}
	cm, err := certificate.NewManager(&certificate.Config{
		Template:         &x509.CertificateRequest{},
		Usages:           []certificatesv1.KeyUsage{},
		CertificateStore: store,
	})
	if err != nil {
		t.Fatalf("Failed to initialize the certificate manager: %v", err)
	}

	tests := []struct {
		name       string
		m          certificate.Manager
		serverAddr string
		caFile     string
		expect     *tls.Config
	}{
		{
			"empty caFile",
			cm,
			"127.0.0.1:80",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := GenTLSConfigUseCertMgrAndCA(tt.m, tt.serverAddr, tt.caFile)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestGenCertPoolUseCA(t *testing.T) {
	tests := []struct {
		name   string
		caFile string
		expect *x509.CertPool
	}{
		{
			"empty caFIle",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := GenCertPoolUseCA(tt.caFile)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}
