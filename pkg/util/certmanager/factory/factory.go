/*
Copyright 2022 The OpenYurt Authors.

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

package factory

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/util/certmanager/store"
)

type IPGetter func() ([]net.IP, error)
type DNSGetter func() ([]string, error)

// CertManagerConfig specifies the attributes of the created CertManager
type CertManagerConfig struct {
	// ComponentName represents the name of the component which will use this CertManager.
	ComponentName string
	// CommonName is CN of cert.
	CommonName string
	// CertDir represents the dir of local file system where the cert related files will be stored.
	CertDir string
	// Organizations is O of cert.
	Organizations []string
	// DNSNames contain a list of DNS names this component will use.
	// Note:
	// If DNSGetter is set and it can get dns names with no error returned,
	// DNSNames will be ignored and what got from DNSGetter will be used.
	DNSNames []string
	// DNSGetter can get dns names at runtime. If no error returned when getting dns names,
	// these dns names will be used in the cert instead of the DNSNames.
	DNSGetter
	// IPs contain a list of IP this component will use.
	// Note:
	// If IPGetter is set and it can get ips with no error returned,
	// IPs will be ignored and what got from IPGetter will be used.
	IPs []net.IP
	// IPGetter can get ips at runtime. If no error returned when getting ips,
	// these ips will be used in the cert instead of the IPs.
	IPGetter
	// SignerName can specified the signer of Kubernetes, which can be one of
	// 1. "kubernetes.io/kube-apiserver-client"
	// 2. "kubernetes.io/kube-apiserver-client-kubelet"
	// 3. "kubernetes.io/kubelet-serving"
	// More details can be found at k8s.io/api/certificates/v1
	SignerName string
	// ForServerUsage indicates the usage of this cert.
	// If set, UsageServerAuth will be used, otherwise UsageClientAuth will be used.
	// Additionally, UsageKeyEncipherment and UsageDigitalSignature are always used.
	ForServerUsage bool
}

// CertManagerFactory knows how to create CertManager for OpenYurt Components.
type CertManagerFactory interface {
	// New function will create the CertManager as what CertManagerConfig specified.
	New(*CertManagerConfig) (certificate.Manager, error)
}

type factory struct {
	clientsetFn certificate.ClientsetFunc
	fileStore   certificate.FileStore
}

func NewCertManagerFactory(clientSet kubernetes.Interface) CertManagerFactory {
	return &factory{
		clientsetFn: func(current *tls.Certificate) (kubernetes.Interface, error) {
			return clientSet, nil
		},
	}
}

func NewCertManagerFactoryWithFnAndStore(clientsetFn certificate.ClientsetFunc, store certificate.FileStore) CertManagerFactory {
	return &factory{
		clientsetFn: clientsetFn,
		fileStore:   store,
	}
}

func (f *factory) New(cfg *CertManagerConfig) (certificate.Manager, error) {
	var err error
	if util.IsNil(f.fileStore) {
		f.fileStore, err = store.NewFileStoreWrapper(cfg.ComponentName, cfg.CertDir, cfg.CertDir, "", "")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the server certificate store: %w", err)
		}
	}

	ips, dnsNames := cfg.IPs, cfg.DNSNames
	getTemplate := func() *x509.CertificateRequest {
		if cfg.IPGetter != nil {
			newIPs, err := cfg.IPGetter()
			if err == nil && len(newIPs) != 0 {
				klog.V(4).Infof("cr template of %s uses ips=%#+v", cfg.ComponentName, newIPs)
				ips = newIPs
			}
			if err != nil {
				klog.Errorf("failed to get ips for %s when preparing cr template, %v", cfg.ComponentName, err)
				return nil
			}
		}
		if cfg.DNSGetter != nil {
			newDNSNames, err := cfg.DNSGetter()
			if err == nil && len(newDNSNames) != 0 {
				klog.V(4).Infof("cr template of %s uses dns names=%#+v", cfg.ComponentName, newDNSNames)
				dnsNames = newDNSNames
			}
			if err != nil {
				klog.Errorf("failed to get dns names for %s when preparing cr template, %v", cfg.ComponentName, err)
				return nil
			}
		}
		return &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   cfg.CommonName,
				Organization: cfg.Organizations,
			},
			DNSNames:    dnsNames,
			IPAddresses: ips,
		}
	}

	usages := []certificatesv1.KeyUsage{
		certificatesv1.UsageKeyEncipherment,
		certificatesv1.UsageDigitalSignature,
	}
	if cfg.ForServerUsage {
		usages = append(usages, certificatesv1.UsageServerAuth)
	} else {
		usages = append(usages, certificatesv1.UsageClientAuth)
	}

	return certificate.NewManager(&certificate.Config{
		ClientsetFn:      f.clientsetFn,
		SignerName:       cfg.SignerName,
		GetTemplate:      getTemplate,
		Usages:           usages,
		CertificateStore: f.fileStore,
		Logf:             klog.Infof,
	})
}
