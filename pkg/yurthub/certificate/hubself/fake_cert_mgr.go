/*
Copyright 2021 The OpenYurt Authors.

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

package hubself

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"path/filepath"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"

	"k8s.io/klog"
)

var (
	defaultCertificatePEM = `-----BEGIN CERTIFICATE-----
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
-----END CERTIFICATE-----`
	defaultKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIBUwIBADANBgkqhkiG9w0BAQEFAASCAT0wggE5AgEAAkEAtBMa7NWpv3BVlKTC
PGO/LEsguKqWHBtKzweMY2CVtAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5M
zP2H5QIDAQABAkAS9BfXab3OKpK3bIgNNyp+DQJKrZnTJ4Q+OjsqkpXvNltPJosf
G8GsiKu/vAt4HGqI3eU77NvRI+mL4MnHRmXBAiEA3qM4FAtKSRBbcJzPxxLEUSwg
XSCcosCktbkXvpYrS30CIQDPDxgqlwDEJQ0uKuHkZI38/SPWWqfUmkecwlbpXABK
iQIgZX08DA8VfvcA5/Xj1Zjdey9FVY6POLXen6RPiabE97UCICp6eUW7ht+2jjar
e35EltCRCjoejRHTuN9TC0uCoVipAiAXaJIx/Q47vGwiw6Y8KXsNU6y54gTbOSxX
54LzHNk/+Q==
-----END RSA PRIVATE KEY-----`
)

type fakeYurtHubCertManager struct {
	certificatePEM   string
	keyPEM           string
	rootDir          string
	hubName          string
	yurthubConifFile string
}

// NewFakeYurtHubCertManager new a YurtCertificateManager instance
func NewFakeYurtHubCertManager(rootDir, yurthubConfigFile, certificatePEM, keyPEM string) (interfaces.YurtCertificateManager, error) {
	hn := projectinfo.GetHubName()
	if len(hn) == 0 {
		hn = hubName
	}
	if len(certificatePEM) == 0 {
		certificatePEM = defaultCertificatePEM
	}
	if len(keyPEM) == 0 {
		keyPEM = defaultKeyPEM
	}

	rd := rootDir
	if len(rd) == 0 {
		rd = filepath.Join(hubRootDir, hn)
	}

	fyc := &fakeYurtHubCertManager{
		certificatePEM:   certificatePEM,
		keyPEM:           keyPEM,
		rootDir:          rd,
		hubName:          hn,
		yurthubConifFile: yurthubConfigFile,
	}

	return fyc, nil
}

// Start create the yurthub.conf file
func (fyc *fakeYurtHubCertManager) Start() {
	dStorage, err := disk.NewDiskStorage(fyc.rootDir)
	if err != nil {
		klog.Errorf("failed to create storage, %v", err)
	}
	fileName := fmt.Sprintf(hubConfigFileName, fyc.hubName)
	yurthubConf := filepath.Join(fyc.rootDir, fileName)
	if err := dStorage.Create(fileName, []byte(fyc.yurthubConifFile)); err != nil {
		klog.Errorf("Unable to create the file %q: %v", yurthubConf, err)
	}
	return
}

// Stop do nothing
func (fyc *fakeYurtHubCertManager) Stop() {}

// Current returns the certificate created by the entered fyc.certificatePEM and fyc.keyPEM
func (fyc *fakeYurtHubCertManager) Current() *tls.Certificate {
	certificate, err := tls.X509KeyPair([]byte(fyc.certificatePEM), []byte(fyc.keyPEM))
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize certificate: %v", err))
	}
	certs, err := x509.ParseCertificates(certificate.Certificate[0])
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize certificate leaf: %v", err))
	}
	certificate.Leaf = certs[0]

	return &certificate
}

// ServerHealthy returns true
func (fyc *fakeYurtHubCertManager) ServerHealthy() bool {
	return true
}

// Update do nothing
func (fyc *fakeYurtHubCertManager) Update(_ *config.YurtHubConfiguration) error {
	return nil
}

// GetCaFile returns the empty path
func (fyc *fakeYurtHubCertManager) GetCaFile() string {
	return ""
}

// GetConfFilePath returns the path of yurtHub config file path
func (fyc *fakeYurtHubCertManager) GetConfFilePath() string {
	return fyc.getHubConfFile()
}

// NotExpired returns true
func (fyc *fakeYurtHubCertManager) NotExpired() bool {
	return fyc.Current() != nil
}

// getHubConfFile returns the path of hub agent conf file.
func (fyc *fakeYurtHubCertManager) getHubConfFile() string {
	return filepath.Join(fyc.rootDir, fmt.Sprintf(hubConfigFileName, fyc.hubName))
}
