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

package manager

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	ipUtils "github.com/openyurtio/openyurt/pkg/util/ip"
	hubCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/kubeletcertificate"
	hubServerCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate/server"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
)

const (
	KubeConfFile   = "/etc/kubernetes/kubelet.conf"
	KubeletCAFile  = "/etc/kubernetes/pki/ca.crt"
	KubeletPemFile = "/var/lib/kubelet/pki/kubelet-client-current.pem"
)

var (
	serverCertNotReadyError          = errors.New("hub server certificate")
	apiServerClientCertNotReadyError = errors.New("APIServer client certificate")
	caCertIsNotReadyError            = errors.New("ca.crt file")

	DefaultWorkDir = filepath.Join("/var/lib", projectinfo.GetHubName())
)

type yurtHubCertManager struct {
	hubCert.YurtClientCertificateManager
	hubCert.YurtServerCertificateManager
}

// NewYurtHubCertManager new a YurtCertificateManager instance
func NewYurtHubCertManager(options *options.YurtHubOptions, remoteServers []*url.URL) (hubCert.YurtCertificateManager, error) {
	var clientCertManager hubCert.YurtClientCertificateManager
	var err error
	var workDir string

	if len(options.RootDir) == 0 {
		workDir = DefaultWorkDir
	} else {
		workDir = options.RootDir
	}

	if options.BootstrapMode == "kubeletcertificate" {
		clientCertManager, err = kubeletcertificate.NewKubeletCertManager(KubeConfFile, KubeletCAFile, KubeletPemFile)
		if err != nil {
			return nil, err
		}
	} else {
		cfg := &token.ClientCertificateManagerConfiguration{
			WorkDir:                  workDir,
			NodeName:                 options.NodeName,
			JoinToken:                options.JoinToken,
			BootstrapFile:            options.BootstrapFile,
			CaCertHashes:             options.CACertHashes,
			YurtHubCertOrganizations: options.YurtHubCertOrganizations,
			RemoteServers:            remoteServers,
			Client:                   options.ClientForTest,
		}
		clientCertManager, err = token.NewYurtHubClientCertManager(cfg)
		if err != nil {
			return nil, err
		}
	}

	// use dummy ip and bind ip as cert IP SANs
	certIPs := ipUtils.RemoveDupIPs([]net.IP{
		net.ParseIP(options.HubAgentDummyIfIP),
		net.ParseIP(options.YurtHubHost),
		net.ParseIP(options.YurtHubProxyHost),
	})
	serverCertManager, err := hubServerCert.NewHubServerCertificateManager(options.ClientForTest, clientCertManager, options.NodeName, filepath.Join(workDir, "pki"), certIPs)
	if err != nil {
		return nil, err
	}

	hubCertManager := &yurtHubCertManager{
		YurtClientCertificateManager: clientCertManager,
		YurtServerCertificateManager: serverCertManager,
	}

	return hubCertManager, nil
}

func (hcm *yurtHubCertManager) Start() {
	hcm.YurtClientCertificateManager.Start()
	hcm.YurtServerCertificateManager.Start()
}

func (hcm *yurtHubCertManager) Stop() {
	hcm.YurtClientCertificateManager.Stop()
	hcm.YurtServerCertificateManager.Stop()
}

func (hcm *yurtHubCertManager) Ready() bool {
	var errs []error
	if hcm.GetAPIServerClientCert() == nil {
		errs = append(errs, apiServerClientCertNotReadyError)
	}

	if len(hcm.YurtClientCertificateManager.GetCAData()) == 0 {
		errs = append(errs, caCertIsNotReadyError)
	}

	if hcm.GetHubServerCert() == nil {
		errs = append(errs, serverCertNotReadyError)
	}

	if len(errs) != 0 {
		klog.Errorf("hub certificates are not ready: %s", utilerrors.NewAggregate(errs).Error())
		return false
	}
	return true
}
