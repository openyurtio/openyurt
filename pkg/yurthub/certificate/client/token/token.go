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

package token

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/certmanager/store"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	hubCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	yurtcertutil "github.com/openyurtio/openyurt/pkg/yurthub/certificate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	YurtHubCSROrg           = "openyurt:yurthub"
	hubPkiDirName           = "pki"
	bootstrapConfigFileName = "bootstrap-hub.conf"
)

var (
	hubConfigFileName = fmt.Sprintf("%s.conf", projectinfo.GetHubName())
)

type ClientCertificateManagerConfiguration struct {
	WorkDir                  string
	NodeName                 string
	JoinToken                string
	BootstrapFile            string
	YurtHubCertOrganizations []string
	RemoteServers            []*url.URL
	Client                   clientset.Interface
}

type yurtHubClientCertManager struct {
	client                     clientset.Interface
	remoteServers              []*url.URL
	apiServerClientCertManager certificate.Manager
	apiServerClientCertStore   certificate.FileStore
	hubRunDir                  string
	hubName                    string
	joinToken                  string
	bootstrapFile              string
	dialer                     *util.Dialer
	caManager                  hubCert.YurtCACertificateManager
}

// NewYurtHubClientCertManager new a YurtCertificateManager instance
func NewYurtHubClientCertManager(cfg *ClientCertificateManagerConfiguration, caManager hubCert.YurtCACertificateManager) (hubCert.YurtClientCertificateManager, error) {
	var err error
	ycm := &yurtHubClientCertManager{
		client:        cfg.Client,
		remoteServers: cfg.RemoteServers,
		hubRunDir:     cfg.WorkDir,
		hubName:       projectinfo.GetHubName(),
		joinToken:     cfg.JoinToken,
		bootstrapFile: cfg.BootstrapFile,
		dialer:        util.NewDialer("hub certificate manager"),
		caManager:     caManager,
	}

	// 1. prepare client certificate manager for connecting remote kube-apiserver by yurthub.
	ycm.apiServerClientCertStore, err = store.NewFileStoreWrapper(ycm.hubName, ycm.getPkiDir(), ycm.getPkiDir(), "", "")
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new client cert store")
	}
	ycm.apiServerClientCertManager, err = ycm.newAPIServerClientCertificateManager(ycm.apiServerClientCertStore, cfg.NodeName, cfg.YurtHubCertOrganizations)
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new apiserver client certificate manager")
	}

	return ycm, nil
}

// Start init certificate manager and certs for hub agent
func (ycm *yurtHubClientCertManager) Start() {
	err := ycm.prepareConfigFile()
	if err != nil {
		klog.Errorf("could not prepare config file, %v", err)
		return
	}

	ycm.apiServerClientCertManager.Start()
}

// prepareConfigAndCaFile is used to create the following two files.
// - /var/lib/yurthub/bootstrap-hub.conf
// - /var/lib/yurthub/yurthub.conf
// if these files already exist, just reuse them.
func (ycm *yurtHubClientCertManager) prepareConfigFile() error {
	var tlsBootstrapCfg *clientcmdapi.Config
	var err error

	// 1. prepare bootstrap file for yurthub
	if len(ycm.bootstrapFile) != 0 {
		tlsBootstrapCfg, err = ycm.prepareBootstrapConfigByFile()
	} else {
		tlsBootstrapCfg, err = ycm.prepareBootstrapConfigByToken()
	}
	if err != nil {
		return err
	}

	// 2. prepare kubeconfig file(/var/lib/yurthub/yurthub.conf) for yurthub
	if exist, err := util.FileExists(ycm.GetHubConfFile()); err != nil {
		return errors.Wrap(err, "couldn't stat hub kubeconfig file")
	} else if exist {
		klog.V(2).Infof("%s file already exists, so reuse it", ycm.GetHubConfFile())
	} else if tlsBootstrapCfg == nil {
		return errors.Errorf("neither boostrap file(%s) nor kubeconfig file(%s) exist when hub agent started", ycm.bootstrapFile, ycm.GetHubConfFile())
	} else {
		hubCfg := createHubConfig(tlsBootstrapCfg, ycm.apiServerClientCertStore.CurrentPath())
		if err = kubeconfigutil.WriteToDisk(ycm.GetHubConfFile(), hubCfg); err != nil {
			return errors.Wrapf(err, "couldn't save %s to disk", hubConfigFileName)
		}
	}

	return nil
}

func (ycm *yurtHubClientCertManager) prepareBootstrapConfigByFile() (*clientcmdapi.Config, error) {
	// 1. load bootstrap config
	if tlsBootstrapCfg, err := clientcmd.LoadFromFile(ycm.getBootstrapConfFile()); err != nil {
		klog.Errorf("maybe hub agent restarted, could not load bootstrap config file(%s), %v.", ycm.getBootstrapConfFile(), err)
		return nil, nil
	} else {
		klog.V(2).Infof("%s file is configured, just use it", ycm.getBootstrapConfFile())
		return tlsBootstrapCfg, nil
	}
}

func (ycm *yurtHubClientCertManager) prepareBootstrapConfigByToken() (*clientcmdapi.Config, error) {
	// in order to keep consistency with old version(with join token),
	// if join token instead of bootstrap-file is set, we will use join token to create boostrap-hub.conf
	// use join token to create bootstrap-hub.conf and will be removed in the future version
	// 1. prepare bootstrap config file(/var/lib/yurthub/bootstrap-hub.conf) for yurthub
	if exist, err := util.FileExists(ycm.getBootstrapConfFile()); err != nil {
		return nil, errors.Wrap(err, "couldn't stat bootstrap config file")
	} else if !exist {
		if tlsBootstrapCfg, err := ycm.retrieveHubBootstrapConfig(ycm.joinToken); err != nil {
			return nil, errors.Wrap(err, "could not retrieve bootstrap config")
		} else {
			return tlsBootstrapCfg, nil
		}
	} else {
		klog.V(2).Infof("%s file already exists, so reuse it", ycm.getBootstrapConfFile())
		if tlsBootstrapCfg, err := clientcmd.LoadFromFile(ycm.getBootstrapConfFile()); err != nil {
			return nil, errors.Wrap(err, "couldn't load bootstrap config file")
		} else {
			return tlsBootstrapCfg, nil
		}
	}
}

// Stop the cert manager loop
func (ycm *yurtHubClientCertManager) Stop() {
	ycm.apiServerClientCertManager.Stop()
}

// UpdateBootstrapConf is used for revising bootstrap conf file by new bearer token.
func (ycm *yurtHubClientCertManager) UpdateBootstrapConf(joinToken string) error {
	_, err := ycm.retrieveHubBootstrapConfig(joinToken)
	return err
}

// getPkiDir returns the directory for storing hub agent pki
func (ycm *yurtHubClientCertManager) getPkiDir() string {
	return filepath.Join(ycm.hubRunDir, hubPkiDirName)
}

// getBootstrapConfFile returns the path of yurthub bootstrap conf file
func (ycm *yurtHubClientCertManager) getBootstrapConfFile() string {
	if len(ycm.bootstrapFile) != 0 {
		return ycm.bootstrapFile
	}
	return filepath.Join(ycm.hubRunDir, bootstrapConfigFileName)
}

// GetHubConfFile returns the path of yurtHub config file path
func (ycm *yurtHubClientCertManager) GetHubConfFile() string {
	return filepath.Join(ycm.hubRunDir, hubConfigFileName)
}

func (ycm *yurtHubClientCertManager) GetAPIServerClientCert() *tls.Certificate {
	return ycm.apiServerClientCertManager.Current()
}

// newAPIServerClientCertificateManager create a certificate manager for yurthub component to prepare client certificate
// that used to proxy requests to remote kube-apiserver.
func (ycm *yurtHubClientCertManager) newAPIServerClientCertificateManager(fileStore certificate.FileStore, nodeName string, hubCertOrganizations []string) (certificate.Manager, error) {
	orgs := []string{YurtHubCSROrg, user.NodesGroup}
	for _, v := range hubCertOrganizations {
		if v != YurtHubCSROrg && v != user.NodesGroup {
			orgs = append(orgs, v)
		}
	}

	return certfactory.NewCertManagerFactoryWithFnAndStore(ycm.generateCertClientFn, fileStore).New(&certfactory.CertManagerConfig{
		ComponentName: ycm.hubName,
		CommonName:    fmt.Sprintf("system:node:%s", nodeName),
		Organizations: orgs,
		SignerName:    certificatesv1.KubeAPIServerClientSignerName,
	})
}

func (ycm *yurtHubClientCertManager) generateCertClientFn(current *tls.Certificate) (clientset.Interface, error) {
	var kubeconfig *restclient.Config
	var err error
	if !yurtutil.IsNil(ycm.client) {
		return ycm.client, nil
	}

	// If we have a valid certificate, use that to fetch CSRs, Otherwise use the bootstrap conf file.
	if current != nil {
		klog.V(2).Infof("use %s config to create csr client", ycm.hubName)
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", ycm.GetHubConfFile())
		if err != nil {
			klog.Errorf("could not load %s kube config(%s), %v", ycm.hubName, ycm.GetHubConfFile(), err)
			return nil, errors.Wrap(err, "could not load hub kubeconfig file")
		}
	} else {
		klog.V(2).Infof("use bootstrap client config to create csr client")
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", ycm.getBootstrapConfFile())
		if err != nil {
			klog.Errorf("could not load bootstrap config in clientFn, %v", err)
			return nil, errors.Wrap(err, "couldn't load hub bootstrap file")
		}
	}

	if kubeconfig == nil {
		return nil, errors.New("kubeconfig for client certificate is not ready")
	}
	kubeconfig.Host = yurtcertutil.FindActiveRemoteServer(net.DialTimeout, ycm.remoteServers).String()
	// re-fix dial for conn management
	kubeconfig.Dial = ycm.dialer.DialContext

	// avoid tcp conn leak: certificate rotated, so close old tcp conn that used to rotate certificate
	klog.V(2).Infof("avoid tcp conn leak, close old tcp conn that used to rotate certificate")
	ycm.dialer.Close(strings.TrimPrefix(kubeconfig.Host, "https://"))

	return clientset.NewForConfig(kubeconfig)
}

func (ycm *yurtHubClientCertManager) retrieveHubBootstrapConfig(joinToken string) (*clientcmdapi.Config, error) {
	// retrieve bootstrap config info from cluster-info configmap by bootstrap token
	serverAddr := yurtcertutil.FindActiveRemoteServer(net.DialTimeout, ycm.remoteServers).Host
	tlsBootstrapCfg := kubeconfigutil.CreateWithToken(
		fmt.Sprintf("https://%s", serverAddr),
		"kubernetes",
		"token-bootstrap-client",
		ycm.caManager.GetCAData(),
		joinToken,
	)
	if err := kubeconfigutil.WriteToDisk(ycm.getBootstrapConfFile(), tlsBootstrapCfg); err != nil {
		klog.Errorf("couldn't save bootstrap-hub.conf to disk, %v", err)
		return nil, errors.Wrap(err, "couldn't save bootstrap-hub.conf to disk")
	}

	return tlsBootstrapCfg, nil
}

func createHubConfig(tlsBootstrapCfg *clientcmdapi.Config, pemPath string) *clientcmdapi.Config {
	cluster := kubeconfigutil.GetClusterFromKubeConfig(tlsBootstrapCfg)

	// Build resulting kubeconfig.
	return &clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   cluster.Server,
			CertificateAuthority:     cluster.CertificateAuthority,
			CertificateAuthorityData: cluster.CertificateAuthorityData,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			ClientCertificate: pemPath,
			ClientKey:         pemPath,
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}
}
