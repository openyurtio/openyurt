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

package hubself

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	certificates "k8s.io/api/certificates/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	certificatesclient "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	hubcert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	HubName                 = "yurthub"
	HubRootDir              = "/var/lib/"
	hubPkiDirName           = "pki"
	hubCaFileName           = "ca.crt"
	hubConfigFileName       = "%s.conf"
	bootstrapConfigFileName = "bootstrap-hub.conf"
	bootstrapUser           = "token-bootstrap-client"
	defaultClusterName      = "kubernetes"
	clusterInfoName         = "cluster-info"
	kubeconfigName          = "kubeconfig"
)

// Register registers a YurtCertificateManager
func Register(cmr *hubcert.CertificateManagerRegistry) {
	cmr.Register(util.YurtHubCertificateManagerName, func(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error) {
		return NewYurtHubCertManager(cfg)
	})
}

type yurtHubCertManager struct {
	remoteServers         []*url.URL
	bootstrapConfStore    storage.Store
	hubClientCertManager  certificate.Manager
	hubClientCertPath     string
	joinToken             string
	caFile                string
	nodeName              string
	rootDir               string
	hubName               string
	kubeletRootCAFilePath string
	kubeletPairFilePath   string
	dialer                *util.Dialer
	stopCh                chan struct{}
}

// NewYurtHubCertManager new a YurtCertificateManager instance
func NewYurtHubCertManager(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error) {
	if cfg == nil || len(cfg.NodeName) == 0 || len(cfg.RemoteServers) == 0 {
		return nil, fmt.Errorf("hub agent configuration is invalid, could not new hub agent cert manager")
	}

	hn := projectinfo.GetHubName()
	if len(hn) == 0 {
		hn = HubName
	}

	rootDir := cfg.RootDir
	if len(rootDir) == 0 {
		rootDir = filepath.Join(HubRootDir, hn)
	}

	ycm := &yurtHubCertManager{
		remoteServers:         cfg.RemoteServers,
		nodeName:              cfg.NodeName,
		joinToken:             cfg.JoinToken,
		kubeletRootCAFilePath: cfg.KubeletRootCAFilePath,
		kubeletPairFilePath:   cfg.KubeletPairFilePath,
		rootDir:               rootDir,
		hubName:               hn,
		dialer:                util.NewDialer("hub certificate manager"),
		stopCh:                make(chan struct{}),
	}

	return ycm, nil
}

// Start init certificate manager and certs for hub agent
func (ycm *yurtHubCertManager) Start() {
	// 1. create ca file for hub certificate manager
	err := ycm.initCaCert()
	if err != nil {
		klog.Errorf("failed to init ca cert, %v", err)
		return
	}
	klog.Infof("use %s ca file to bootstrap %s", ycm.caFile, ycm.hubName)

	// 2. create bootstrap config file for hub certificate manager
	err = ycm.initBootstrap()
	if err != nil {
		klog.Errorf("failed to init bootstrap %v", err)
		return
	}

	// 3. create client certificate manager for hub certificate manager
	err = ycm.initClientCertificateManager()
	if err != nil {
		klog.Errorf("failed to init client cert manager, %v", err)
		return
	}

	// 4. create hub config file
	err = ycm.initHubConf()
	if err != nil {
		klog.Errorf("failed to init hub config, %v", err)
		return
	}
}

// Stop the cert manager loop
func (ycm *yurtHubCertManager) Stop() {
	if ycm.hubClientCertManager != nil {
		ycm.hubClientCertManager.Stop()
	}
}

// Current returns the currently selected certificate from the certificate manager
func (ycm *yurtHubCertManager) Current() *tls.Certificate {
	if ycm.hubClientCertManager != nil {
		return ycm.hubClientCertManager.Current()
	}

	return nil
}

// ServerHealthy returns true if the cert manager believes the server is currently alive.
func (ycm *yurtHubCertManager) ServerHealthy() bool {
	if ycm.hubClientCertManager != nil {
		return ycm.hubClientCertManager.ServerHealthy()
	}

	return false
}

// Update update bootstrap conf file by new bearer token.
func (ycm *yurtHubCertManager) Update(cfg *config.YurtHubConfiguration) error {
	if cfg == nil {
		return nil
	}

	err := ycm.updateBootstrapConfFile(cfg.JoinToken)
	if err != nil {
		klog.Errorf("could not update hub agent bootstrap config file, %v", err)
		return err
	}

	return nil
}

// GetCaFile returns the path of ca file
func (ycm *yurtHubCertManager) GetCaFile() string {
	return ycm.caFile
}

// GetConfFilePath returns the path of yurtHub config file path
func (ycm *yurtHubCertManager) GetConfFilePath() string {
	return ycm.getHubConfFile()
}

// NotExpired returns hub client cert is expired or not.
// True: not expired
// False: expired
func (ycm *yurtHubCertManager) NotExpired() bool {
	return ycm.Current() != nil
}

// initCaCert create ca file for hub certificate manager
func (ycm *yurtHubCertManager) initCaCert() error {
	caFile := ycm.getCaFile()
	ycm.caFile = caFile

	if exists, err := util.FileExists(caFile); exists {
		klog.Infof("%s file already exists, so skip to create ca file", caFile)
		return nil
	} else if err != nil {
		klog.Errorf("could not stat ca file %s, %v", caFile, err)
		return err
	} else {
		klog.Infof("%s file not exists, so create it", caFile)
	}

	insecureRestConfig, err := createInsecureRestClientConfig(ycm.remoteServers[0])
	if err != nil {
		klog.Errorf("could not create insecure rest config, %v", err)
		return err
	}

	insecureClient, err := clientset.NewForConfig(insecureRestConfig)
	if err != nil {
		klog.Errorf("could not new insecure client, %v", err)
		return err
	}

	// make sure configMap kube-public/cluster-info in k8s cluster beforehand
	insecureClusterInfo, err := insecureClient.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.Background(), clusterInfoName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get cluster-info configmap, %v", err)
		return err
	}

	kubeconfigStr, ok := insecureClusterInfo.Data[kubeconfigName]
	if !ok || len(kubeconfigStr) == 0 {
		return fmt.Errorf("no kubeconfig in cluster-info configmap of kube-public namespace")
	}

	kubeConfig, err := clientcmd.Load([]byte(kubeconfigStr))
	if err != nil {
		return fmt.Errorf("could not load kube config string, %v", err)
	}

	if len(kubeConfig.Clusters) != 1 {
		return fmt.Errorf("more than one cluster setting in cluster-info configmap")
	}

	var clusterCABytes []byte
	for _, cluster := range kubeConfig.Clusters {
		clusterCABytes = cluster.CertificateAuthorityData
	}

	if err := certutil.WriteCert(caFile, clusterCABytes); err != nil {
		klog.Errorf("could not write %s ca cert, %v", ycm.hubName, err)
		return err
	}

	return nil
}

// initBootstrap create bootstrap config file for hub certificate manager
func (ycm *yurtHubCertManager) initBootstrap() error {
	bootstrapConfStore, err := disk.NewDiskStorage(ycm.rootDir)
	if err != nil {
		klog.Errorf("could not new disk storage for bootstrap conf file, %v", err)
		return err
	}
	ycm.bootstrapConfStore = bootstrapConfStore

	contents, err := ycm.bootstrapConfStore.Get(bootstrapConfigFileName)
	if err == storage.ErrStorageNotFound {
		klog.Infof("%s bootstrap conf file does not exist, so create it", ycm.hubName)
		return ycm.createBootstrapConfFile(ycm.joinToken)
	} else if err != nil {
		klog.Infof("could not get bootstrap conf file, %v", err)
		return err
	} else if len(contents) == 0 {
		klog.Infof("%s bootstrap conf file does not exist, so create it", ycm.hubName)
		return ycm.createBootstrapConfFile(ycm.joinToken)
	} else {
		klog.Infof("%s bootstrap conf file already exists, skip init bootstrap", ycm.hubName)
		return nil
	}
}

// initClientCertificateManager init hub client certificate manager
func (ycm *yurtHubCertManager) initClientCertificateManager() error {
	s, err := certificate.NewFileStore(ycm.hubName, ycm.getPkiDir(), ycm.getPkiDir(), "", "")
	if err != nil {
		klog.Errorf("failed to init %s client cert store, %v", ycm.hubName, err)
		return err

	}
	ycm.hubClientCertPath = s.CurrentPath()

	m, err := certificate.NewManager(&certificate.Config{
		ClientFn:   ycm.generateCertClientFn,
		SignerName: certificates.KubeAPIServerClientKubeletSignerName,
		Template: &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   fmt.Sprintf("system:node:%s", ycm.nodeName),
				Organization: []string{"system:nodes"},
			},
		},
		Usages: []certificates.KeyUsage{
			certificates.UsageDigitalSignature,
			certificates.UsageKeyEncipherment,
			certificates.UsageClientAuth,
		},

		CertificateStore: s,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize client certificate manager: %v", err)
	}
	ycm.hubClientCertManager = m
	m.Start()

	return nil
}

// getBootstrapClientConfig get rest client config from bootstrap conf file.
// and when no bearer token in bootstrap conf file, kubelet.conf will be used instead.
func (ycm *yurtHubCertManager) getBootstrapClientConfig(healthyServer *url.URL) (*restclient.Config, error) {
	restCfg, err := util.LoadRESTClientConfig(ycm.getBootstrapConfFile())
	if err != nil {
		klog.Errorf("could not load rest client config from bootstrap file(%s), %v", ycm.getBootstrapConfFile(), err)
		return nil, err
	}

	if len(restCfg.BearerToken) != 0 {
		klog.V(3).Infof("join token is set for bootstrap client config")
		// re-fix healthy host for bootstrap client config
		restCfg.Host = healthyServer.String()
		return restCfg, nil
	}

	klog.Infof("no join token, so use kubelet config to bootstrap hub")
	// use kubelet.conf to bootstrap hub agent
	return util.LoadKubeletRestClientConfig(healthyServer, ycm.kubeletRootCAFilePath, ycm.kubeletPairFilePath)
}

func (ycm *yurtHubCertManager) generateCertClientFn(current *tls.Certificate) (certificatesclient.CertificateSigningRequestInterface, error) {
	var cfg *restclient.Config
	var healthyServer *url.URL
	hubConfFile := ycm.getHubConfFile()

	_ = wait.PollInfinite(30*time.Second, func() (bool, error) {
		healthyServer = ycm.remoteServers[0]
		if healthyServer == nil {
			klog.V(3).Infof("all of remote servers are unhealthy, just wait")
			return false, nil
		}

		// If we have a valid certificate, use that to fetch CSRs.
		// Otherwise use the bootstrap conf file.
		if current != nil {
			klog.V(3).Infof("use %s config to create csr client", ycm.hubName)
			// use the valid certificate
			kubeConfig, err := util.LoadRESTClientConfig(hubConfFile)
			if err != nil {
				klog.Errorf("could not load %s kube config, %v", ycm.hubName, err)
				return false, nil
			}

			// re-fix healthy host for cert manager
			kubeConfig.Host = healthyServer.String()
			cfg = kubeConfig
		} else {
			klog.V(3).Infof("use bootstrap client config to create csr client")
			// bootstrap is updated
			bootstrapClientConfig, err := ycm.getBootstrapClientConfig(healthyServer)
			if err != nil {
				klog.Errorf("could not load bootstrap config in clientFn, %v", err)
				return false, nil
			}

			cfg = bootstrapClientConfig
		}

		if cfg != nil {
			klog.V(3).Infof("bootstrap client config: %#+v", cfg)
			// re-fix dial for conn management
			cfg.Dial = ycm.dialer.DialContext
		}
		return true, nil
	})

	// avoid tcp conn leak: certificate rotated, so close old tcp conn that used to rotate certificate
	klog.V(2).Infof("avoid tcp conn leak, close old tcp conn that used to rotate certificate")
	ycm.dialer.Close(strings.Trim(cfg.Host, "https://"))

	client, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return client.CertificatesV1beta1().CertificateSigningRequests(), nil
}

// initHubConf init hub agent conf file.
func (ycm *yurtHubCertManager) initHubConf() error {
	hubConfFile := ycm.getHubConfFile()
	if exists, err := util.FileExists(hubConfFile); exists {
		klog.Infof("%s config file already exists, skip init config file", ycm.hubName)
		return nil
	} else if err != nil {
		klog.Errorf("could not stat %s config file %s, %v", ycm.hubName, hubConfFile, err)
		return err
	} else {
		klog.Infof("%s file not exists, so create it", hubConfFile)
	}

	bootstrapClientConfig, err := util.LoadRESTClientConfig(ycm.getBootstrapConfFile())
	if err != nil {
		klog.Errorf("could not load bootstrap client config for init cert store, %v", err)
		return err
	}
	hubClientConfig := restclient.AnonymousClientConfig(bootstrapClientConfig)
	hubClientConfig.KeyFile = ycm.hubClientCertPath
	hubClientConfig.CertFile = ycm.hubClientCertPath
	err = util.CreateKubeConfigFile(hubClientConfig, hubConfFile)
	if err != nil {
		klog.Errorf("could not create %s config file, %v", ycm.hubName, err)
		return err
	}

	return nil
}

// getPkiDir returns the directory for storing hub agent pki
func (ycm *yurtHubCertManager) getPkiDir() string {
	return filepath.Join(ycm.rootDir, hubPkiDirName)
}

// getCaFile returns the path of ca file
func (ycm *yurtHubCertManager) getCaFile() string {
	return filepath.Join(ycm.getPkiDir(), hubCaFileName)
}

// getBootstrapConfFile returns the path of bootstrap conf file
func (ycm *yurtHubCertManager) getBootstrapConfFile() string {
	return filepath.Join(ycm.rootDir, bootstrapConfigFileName)
}

// getHubConfFile returns the path of hub agent conf file.
func (ycm *yurtHubCertManager) getHubConfFile() string {
	return filepath.Join(ycm.rootDir, fmt.Sprintf(hubConfigFileName, ycm.hubName))
}

// createBasic create basic client cmd config
func createBasic(apiServerAddr string, caCert []byte) *clientcmdapi.Config {
	contextName := fmt.Sprintf("%s@%s", bootstrapUser, defaultClusterName)

	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			defaultClusterName: {
				Server:                   apiServerAddr,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  defaultClusterName,
				AuthInfo: bootstrapUser,
			},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		CurrentContext: contextName,
	}
}

// createInsecureRestClientConfig create insecure rest client config.
func createInsecureRestClientConfig(remoteServer *url.URL) (*restclient.Config, error) {
	if remoteServer == nil {
		return nil, fmt.Errorf("no healthy remote server")
	}
	cfg := createBasic(remoteServer.String(), []byte{})
	cfg.Clusters[defaultClusterName].InsecureSkipTLSVerify = true

	restConfig, err := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create insecure rest client configuration, %v", err)
	}
	return restConfig, nil
}

// createBootstrapConf create bootstrap conf info
func createBootstrapConf(apiServerAddr, caFile, joinToken string) *clientcmdapi.Config {
	if len(apiServerAddr) == 0 || len(caFile) == 0 {
		return nil
	}

	exists, err := util.FileExists(caFile)
	if err != nil || !exists {
		klog.Errorf("ca file(%s) is not exist, %v", caFile, err)
		return nil
	}

	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		klog.Errorf("could not read ca file(%s), %v", caFile, err)
		return nil
	}

	cfg := createBasic(apiServerAddr, caCert)
	cfg.AuthInfos[bootstrapUser] = &clientcmdapi.AuthInfo{Token: joinToken}

	return cfg
}

// createBootstrapConfFile create bootstrap conf file
func (ycm *yurtHubCertManager) createBootstrapConfFile(joinToken string) error {
	remoteServer := ycm.remoteServers[0]
	if remoteServer == nil || len(remoteServer.Host) == 0 {
		return fmt.Errorf("no healthy server for create bootstrap conf file")
	}

	bootstrapConfig := createBootstrapConf(remoteServer.String(), ycm.caFile, joinToken)
	if bootstrapConfig == nil {
		return fmt.Errorf("could not create bootstrap config for %s", ycm.hubName)
	}

	content, err := clientcmd.Write(*bootstrapConfig)
	if err != nil {
		klog.Errorf("could not create bootstrap config into bytes got error, %v", err)
		return err
	}

	err = ycm.bootstrapConfStore.Update(bootstrapConfigFileName, content)
	if err != nil {
		klog.Errorf("could not create bootstrap conf file(%s), %v", ycm.getBootstrapConfFile(), err)
		return err
	}

	return nil
}

// updateBootstrapConfFile update bearer token in bootstrap conf file
func (ycm *yurtHubCertManager) updateBootstrapConfFile(joinToken string) error {
	if len(joinToken) == 0 {
		return fmt.Errorf("joinToken should not be empty when update bootstrap conf file")
	}

	var curKubeConfig *clientcmdapi.Config
	if existed, _ := util.FileExists(ycm.getBootstrapConfFile()); !existed {
		klog.Infof("bootstrap conf file not exists(maybe deleted unintentionally), so create a new one")
		return ycm.createBootstrapConfFile(joinToken)
	}

	curKubeConfig, err := util.LoadKubeConfig(ycm.getBootstrapConfFile())
	if err != nil || curKubeConfig == nil {
		klog.Errorf("could not get current bootstrap config for %s, %v", ycm.hubName, err)
		return fmt.Errorf("could not load bootstrap conf file(%s), %v", ycm.getBootstrapConfFile(), err)
	}

	if curKubeConfig.AuthInfos[bootstrapUser] != nil {
		if curKubeConfig.AuthInfos[bootstrapUser].Token == joinToken {
			klog.Infof("join token for %s bootstrap conf file is not changed", ycm.hubName)
			return nil
		}
	}

	curKubeConfig.AuthInfos[bootstrapUser] = &clientcmdapi.AuthInfo{Token: joinToken}
	content, err := clientcmd.Write(*curKubeConfig)
	if err != nil {
		klog.Errorf("could not update bootstrap config into bytes, %v", err)
		return err
	}

	err = ycm.bootstrapConfStore.Update(bootstrapConfigFileName, content)
	if err != nil {
		klog.Errorf("could not update bootstrap config, %v", err)
		return err
	}

	return nil
}
