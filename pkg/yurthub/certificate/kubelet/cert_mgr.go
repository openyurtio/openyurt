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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	certVerifyDuration = 30 * time.Minute
)

// Register registers a YurtCertificateManager
func Register(cmr *certificate.CertificateManagerRegistry) {
	cmr.Register(util.KubeletCertificateManagerName, func(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error) {
		return NewKubeletCertManager(cfg, 0)
	})
}

type kubeletCertManager struct {
	certAccessLock     sync.RWMutex
	pairFile           string
	cert               *tls.Certificate
	stopCh             chan struct{}
	remoteServers      []*url.URL
	caFile             string
	certVerifyDuration time.Duration
	stopped            bool
}

// NewKubeletCertManager creates a YurtCertificateManager
func NewKubeletCertManager(cfg *config.YurtHubConfiguration, period time.Duration) (interfaces.YurtCertificateManager, error) {
	var cert *tls.Certificate
	pairFile := cfg.KubeletPairFilePath
	if cfg == nil || len(cfg.RemoteServers) == 0 {
		return nil, fmt.Errorf("hub configuration is invalid")
	}

	if period == time.Duration(0) {
		period = certVerifyDuration
	}

	if pairFileExists, err := util.FileExists(pairFile); err != nil {
		return nil, err
	} else if pairFileExists {
		klog.Infof("Loading cert/key pair from %q.", pairFile)
		cert, err = loadFile(pairFile)
		if err != nil {
			return nil, err
		}
	}

	return &kubeletCertManager{
		pairFile:           pairFile,
		cert:               cert,
		remoteServers:      cfg.RemoteServers,
		caFile:             cfg.KubeletRootCAFilePath,
		certVerifyDuration: period,
		stopCh:             make(chan struct{}),
	}, nil
}

// Stop stop cert manager
func (kcm *kubeletCertManager) Stop() {
	kcm.certAccessLock.Lock()
	defer kcm.certAccessLock.Unlock()
	if kcm.stopped {
		return
	}
	close(kcm.stopCh)
	kcm.stopped = true
}

// Start runs certificate reloading after rotation
func (kcm *kubeletCertManager) Start() {
	go wait.Until(func() {
		newCert, err := loadFile(kcm.pairFile)
		if err != nil {
			klog.Errorf("failed to load cert file %s, %v", kcm.pairFile, err)
			return
		}

		certChanged := true
		kcm.certAccessLock.RLock()
		if kcm.cert.Leaf.NotAfter.Equal(newCert.Leaf.NotAfter) {
			certChanged = false
		}
		kcm.certAccessLock.RUnlock()

		if certChanged {
			klog.Infof("cert file %s is updated", kcm.pairFile)
			kcm.updateCert(newCert)
		}

	}, kcm.certVerifyDuration, kcm.stopCh)
}

// Current get the current certificate
func (kcm *kubeletCertManager) Current() *tls.Certificate {
	kcm.certAccessLock.RLock()
	defer kcm.certAccessLock.RUnlock()
	return kcm.cert
}

// ServerHealthy always returns true
func (kcm *kubeletCertManager) ServerHealthy() bool {
	return true
}

// GetCaFile get an ca file
func (kcm *kubeletCertManager) GetCaFile() string {
	return kcm.caFile
}

// GetConfFilePath get an kube-config file path, but the kubelet mode just using the ca and pair, so return empty
func (kcm *kubeletCertManager) GetConfFilePath() string {
	return ""
}

func (kcm *kubeletCertManager) NotExpired() bool {
	kcm.certAccessLock.RLock()
	defer kcm.certAccessLock.RUnlock()
	if kcm.cert == nil || kcm.cert.Leaf == nil || time.Now().After(kcm.cert.Leaf.NotAfter) {
		klog.V(2).Infof("Current certificate is expired.")
		return false
	}
	return true
}

func (kcm *kubeletCertManager) updateCert(c *tls.Certificate) {
	kcm.certAccessLock.Lock()
	defer kcm.certAccessLock.Unlock()
	kcm.cert = c
}

// Update do nothing
func (kcm *kubeletCertManager) Update(_ *config.YurtHubConfiguration) error {
	return nil
}

func loadFile(pairFile string) (*tls.Certificate, error) {
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
