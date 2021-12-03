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

package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
)

// hubServerCertMgr is the certificate manager for server usage, which will cache the old hub-server-certificate,
type hubServerCertMgr struct {
	// the path of old hub server certificate
	oldCertFile string
	// the cached old hub server certificate
	oldCert *tls.Certificate
	// the actual hub certMgr
	hubCertMgr certificate.Manager
}

// newHubServerCertMgr creates a certificate manager for the yurthub-server
func newHubServerCertMgr(certDir string, hubCertMgr certificate.Manager) hubServerCertMgr {
	var oldCertFile string
	var oldCert *tls.Certificate

	store, err := certificate.NewFileStore(fmt.Sprintf("%s-server", projectinfo.GetHubName()), certDir, certDir, "", "")
	if err != nil {
		oldCertFile = ""
		oldCert = nil
	} else {
		oldCertFile = store.CurrentPath()
		oldCert, err = store.Current()
		if err != nil {
			oldCert = nil
		}
	}

	return hubServerCertMgr{
		oldCertFile: oldCertFile,
		oldCert:     oldCert,
		hubCertMgr:  hubCertMgr,
	}
}

// Current is used to obtain the certificate for hub server usage.
// If the current certificate of hub certMgr is invalid (if the key usages of certificate doesn't contain the UsageServerAuth),
// the old certificate in the yurthub-server-current.pem will be used.
func (scm *hubServerCertMgr) Current() *tls.Certificate {
	cert := scm.hubCertMgr.Current()
	if cert == nil {
		return &tls.Certificate{Certificate: nil}
	}

	if hasServerAuthUsage(cert) {
		return cert
	}

	klog.V(3).Infof("the %s certificate is not valid for server, try to use the old server certificate in %s",
		projectinfo.GetHubName(), scm.oldCertFile)
	if scm.oldCert != nil && time.Now().Before(scm.oldCert.Leaf.NotAfter) {
		return scm.oldCert
	} else {
		klog.V(3).Infof("failed to get the old %s server certificate, wait for the new hub certificate", projectinfo.GetHubName())
	}
	return &tls.Certificate{Certificate: nil}
}

// GenTLSConfigUseCertMgr generates a TLS configuration by using the given certificate manager
func GenTLSConfigUseCertMgr(m interfaces.YurtCertificateManager, certDir string) (*tls.Config, error) {
	// generate the TLS configuration based on the latest certificate
	rootCert, err := GenCertPoolUseCA(m.GetCaFile())
	if err != nil {
		klog.Errorf("could not generate a x509 CertPool based on the given CA file, %v", err)
		return nil, err
	}

	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
		ClientCAs:  rootCert,
		ClientAuth: tls.VerifyClientCertIfGiven,
	}

	scm := newHubServerCertMgr(certDir, m)
	tlsConfig.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return scm.Current(), nil }

	return tlsConfig, nil
}

// GenCertPoolUseCA generates a x509 CertPool based on the given CA file
func GenCertPoolUseCA(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, errors.New("CA file is not set")
	}

	if _, err := os.Stat(caFile); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("CA file(%s) doesn't exist", caFile)
		}
		return nil, fmt.Errorf("fail to stat the CA file(%s): %s", caFile, err)
	}

	caData, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caData)
	return certPool, nil
}

// hasServerAuthUsage checks whether the certificate is valid or not
func hasServerAuthUsage(cert *tls.Certificate) bool {
	if cert == nil {
		return false
	}
	// if the key usages of certificate doesn't contain the UsageServerAuth, it means the cert is invalid
	for _, v := range cert.Leaf.ExtKeyUsage {
		if v == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}
