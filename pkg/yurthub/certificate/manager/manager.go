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
	"os"
	"path/filepath"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	ipUtils "github.com/openyurtio/openyurt/pkg/util/ip"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	hubCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/ca"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/client"
	hubServerCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate/server"
)

var (
	serverCertNotReadyError          = errors.New("hub server certificate")
	apiServerClientCertNotReadyError = errors.New("APIServer client certificate")
	caCertIsNotReadyError            = errors.New("ca.crt file")

	DefaultWorkDir = filepath.Join("/var/lib", projectinfo.GetHubName())
)

type yurtHubCertManager struct {
	hubCert.YurtCACertificateManager
	hubCert.YurtClientCertificateManager
	hubCert.YurtServerCertificateManager
}

// NewYurtHubCertManager new a YurtCertificateManager instance
func NewYurtHubCertManager(options *options.YurtHubOptions, remoteServers []*url.URL) (hubCert.YurtCertificateManager, error) {
	var workDir string

	if len(options.RootDir) == 0 {
		workDir = DefaultWorkDir
	} else {
		workDir = options.RootDir
	}

	caManager, err := ca.NewCAManager(options.ClientForTest, options.BootstrapMode, workDir, options.BootstrapFile, options.JoinToken, remoteServers, options.CACertHashes)
	if err != nil {
		return nil, err
	}

	clientCertManager, err := client.NewYurtClientCertificateManager(options, remoteServers, caManager, workDir)
	if err != nil {
		return nil, err
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
		YurtCACertificateManager:     caManager,
		YurtClientCertificateManager: clientCertManager,
		YurtServerCertificateManager: serverCertManager,
	}

	// verify that need to clean up stale certificates or not based on server addresses.
	hubCertManager.verifyServerAddrOrCleanup(remoteServers, workDir)
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

	if len(hcm.YurtCACertificateManager.GetCAData()) == 0 {
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

func removeDirContents(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, d := range files {
		err = os.RemoveAll(filepath.Join(dir, d.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}

func (hcm *yurtHubCertManager) verifyServerAddrOrCleanup(servers []*url.URL, workDir string) {
	if cfg, err := clientcmd.LoadFromFile(hcm.GetHubConfFile()); err == nil {
		cluster := kubeconfigutil.GetClusterFromKubeConfig(cfg)
		if serverURL, err := url.Parse(cluster.Server); err != nil {
			klog.Errorf("couldn't get server info from %s, %v", hcm.GetHubConfFile(), err)
		} else {
			for i := range servers {
				if servers[i].Host == serverURL.Host {
					klog.Infof("no change in apiServer address: %s", cluster.Server)
					return
				}
			}
		}

		klog.Infof("config for apiServer %s found, need to recycle for new server %v", cluster.Server, servers)
		removeDirContents(workDir)
	}
}
