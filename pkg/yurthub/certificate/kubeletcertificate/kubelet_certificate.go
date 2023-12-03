/*
Copyright 2023 The OpenYurt Authors.

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

package kubeletcertificate

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	KubeConfNotExistErr   = errors.New("/etc/kubernetes/kubelet.conf file doesn't exist")
	KubeletCANotExistErr  = errors.New("/etc/kubernetes/pki/ca.crt file doesn't exist")
	KubeletPemNotExistErr = errors.New("/var/lib/kubelet/pki/kubelet-client-current.pem file doesn't exist")
)

type kubeletCertManager struct {
	kubeConfFile   string
	kubeletCAFile  string
	kubeletPemFile string
	cert           *tls.Certificate
	caData         []byte
}

func NewKubeletCertManager(kubeConfFile, kubeletCAFile, kubeletPemFile string) (certificate.YurtClientCertificateManager, error) {
	if exist, _ := util.FileExists(kubeConfFile); !exist {
		return nil, KubeConfNotExistErr
	}

	if exist, _ := util.FileExists(kubeletCAFile); !exist {
		return nil, KubeletCANotExistErr
	}
	caData, err := os.ReadFile(kubeletCAFile)
	if err != nil {
		return nil, err
	}

	if exist, _ := util.FileExists(kubeletPemFile); !exist {
		return nil, KubeletPemNotExistErr
	}

	cert, err := loadFile(kubeletPemFile)
	if err != nil {
		return nil, err
	}

	return &kubeletCertManager{
		kubeConfFile:   kubeConfFile,
		kubeletCAFile:  kubeletCAFile,
		kubeletPemFile: kubeletPemFile,
		cert:           cert,
		caData:         caData,
	}, nil
}

func (kcm *kubeletCertManager) Start() {
	// do nothing
}

func (kcm *kubeletCertManager) Stop() {
	// do nothing
}

func (kcm *kubeletCertManager) UpdateBootstrapConf(_ string) error {
	return nil
}

func (kcm *kubeletCertManager) GetHubConfFile() string {
	return kcm.kubeConfFile
}

func (kcm *kubeletCertManager) GetCAData() []byte {
	return kcm.caData
}

func (kcm *kubeletCertManager) GetCaFile() string {
	return kcm.kubeletCAFile
}

func (kcm *kubeletCertManager) GetAPIServerClientCert() *tls.Certificate {
	if kcm.cert != nil && kcm.cert.Leaf != nil && !time.Now().After(kcm.cert.Leaf.NotAfter) {
		return kcm.cert
	}

	klog.Warningf("current certificate: %s is expired, reload it", kcm.kubeletPemFile)
	cert, err := loadFile(kcm.kubeletPemFile)
	if err != nil {
		klog.Errorf("could not load client certificate(%s), %v", kcm.kubeletPemFile, err)
		return nil
	}
	kcm.cert = cert
	return kcm.cert
}

func loadFile(pairFile string) (*tls.Certificate, error) {
	// LoadX509KeyPair knows how to parse combined cert and private key from
	// the same file.
	cert, err := tls.LoadX509KeyPair(pairFile, pairFile)
	if err != nil {
		return nil, fmt.Errorf("could not convert data from %q into cert/key pair: %v", pairFile, err)
	}
	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %v", err)
	}
	cert.Leaf = certs[0]
	return &cert, nil
}
