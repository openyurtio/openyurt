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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/certmanager/store"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/util/token"
	hubCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	YurtHubCSROrg           = "openyurt:yurthub"
	DefaultRootDir          = "/var/lib"
	hubPkiDirName           = "pki"
	hubCaFileName           = "ca.crt"
	bootstrapConfigFileName = "bootstrap-hub.conf"
)

var (
	hubConfigFileName                = fmt.Sprintf("%s.conf", projectinfo.GetHubName())
	serverCertNotReadyError          = errors.New("hub server certificate")
	apiServerClientCertNotReadyError = errors.New("APIServer client certificate")
	caCertIsNotReadyError            = errors.New("ca.crt file")
)

type yurtHubCertManager struct {
	client                     clientset.Interface
	remoteServers              []*url.URL
	caCertHashes               []string
	apiServerClientCertManager certificate.Manager
	hubServerCertManager       certificate.Manager
	apiServerClientCertStore   certificate.FileStore
	serverCertStore            certificate.FileStore
	hubRunDir                  string
	hubName                    string
	joinToken                  string
	dialer                     *util.Dialer
}

// NewYurtHubCertManager new a YurtCertificateManager instance
func NewYurtHubCertManager(client clientset.Interface, cfg *config.YurtHubConfiguration, stopCh <-chan struct{}) (hubCert.YurtCertificateManager, error) {
	var err error

	hubRunDir := cfg.RootDir
	if len(cfg.RootDir) == 0 {
		hubRunDir = filepath.Join(DefaultRootDir, projectinfo.GetHubName())
	}

	ycm := &yurtHubCertManager{
		client:        client,
		remoteServers: cfg.RemoteServers,
		hubRunDir:     hubRunDir,
		hubName:       projectinfo.GetHubName(),
		joinToken:     cfg.JoinToken,
		caCertHashes:  cfg.CaCertHashes,
		dialer:        util.NewDialer("hub certificate manager"),
	}

	// 1. verify that need to clean up stale certificates or not based on server addresses.
	ycm.verifyServerAddrOrCleanup(cfg.RemoteServers)

	// 2. prepare client certificate manager for connecting remote kube-apiserver by yurthub.
	ycm.apiServerClientCertStore, err = store.NewFileStoreWrapper(ycm.hubName, ycm.getPkiDir(), ycm.getPkiDir(), "", "")
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new client cert store")
	}
	ycm.apiServerClientCertManager, err = ycm.newAPIServerClientCertificateManager(ycm.apiServerClientCertStore, cfg.NodeName, cfg.YurtHubCertOrganizations)
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new apiserver client certificate manager")
	}

	// 3. prepare yurthub server certificate manager
	ycm.serverCertStore, err = store.NewFileStoreWrapper(fmt.Sprintf("%s-server", ycm.hubName), ycm.getPkiDir(), ycm.getPkiDir(), "", "")
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new hub server cert store")
	}

	ycm.hubServerCertManager, err = ycm.newHubServerCertificateManager(ycm.serverCertStore, cfg.NodeName, cfg.CertIPs)
	if err != nil {
		return ycm, errors.Wrap(err, "couldn't new hub server certificate manager")
	}

	return ycm, nil
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

func (ycm *yurtHubCertManager) verifyServerAddrOrCleanup(servers []*url.URL) {
	if cfg, err := clientcmd.LoadFromFile(ycm.getBootstrapConfFile()); err == nil {
		cluster := kubeconfigutil.GetClusterFromKubeConfig(cfg)
		if serverURL, err := url.Parse(cluster.Server); err != nil {
			klog.Errorf("couldn't get server info from %s, %v", ycm.getBootstrapConfFile(), err)
		} else {
			for i := range servers {
				if servers[i].Host == serverURL.Host {
					klog.Infof("apiServer name %s not changed", cluster.Server)
					return
				}
			}
		}

		klog.Infof("config for apiServer %s found, need to recycle for new server %v", cluster.Server, servers)
		removeDirContents(ycm.hubRunDir)
	}
}

// Start init certificate manager and certs for hub agent
func (ycm *yurtHubCertManager) Start() {
	err := ycm.prepareConfigAndCaFile()
	if err != nil {
		klog.Errorf("failed to prepare config and ca file, %v", err)
		return
	}

	ycm.apiServerClientCertManager.Start()
	ycm.hubServerCertManager.Start()
}

// prepareConfigAndCaFile is used to create the following three files.
// - /var/lib/yurthub/bootstrap-hub.conf
// - /var/lib/yurthub/yurthub.conf
// - /var/lib/yurthub/pki/ca.crt
// if these files already exist, just reuse them.
func (ycm *yurtHubCertManager) prepareConfigAndCaFile() error {
	var tlsBootstrapCfg *clientcmdapi.Config

	// 1. prepare bootstrap config file(/var/lib/yurthub/bootstrap-hub.conf) for yurthub
	if exist, err := util.FileExists(ycm.getBootstrapConfFile()); err != nil {
		return errors.Wrap(err, "couldn't stat bootstrap config file")
	} else if !exist {
		if tlsBootstrapCfg, err = ycm.retrieveHubBootstrapConfig(ycm.joinToken); err != nil {
			return errors.Wrap(err, "failed to retrieve bootstrap config")
		}
	} else {
		klog.V(2).Infof("%s file already exists, so reuse it", ycm.getBootstrapConfFile())
		if tlsBootstrapCfg, err = clientcmd.LoadFromFile(ycm.getBootstrapConfFile()); err != nil {
			return errors.Wrap(err, "couldn't load bootstrap config file")
		}
	}

	// 2. prepare kubeconfig file(/var/lib/yurthub/yurthub.conf) for yurthub
	if exist, err := util.FileExists(ycm.GetHubConfFile()); err != nil {
		return errors.Wrap(err, "couldn't stat hub kubeconfig file")
	} else if !exist {
		hubCfg := createHubConfig(tlsBootstrapCfg, ycm.apiServerClientCertStore.CurrentPath())
		if err = kubeconfigutil.WriteToDisk(ycm.GetHubConfFile(), hubCfg); err != nil {
			return errors.Wrapf(err, "couldn't save %s to disk", hubConfigFileName)
		}
	} else {
		klog.V(2).Infof("%s file already exists, so reuse it", ycm.GetHubConfFile())
	}

	// 3. prepare ca.crt file(/var/lib/yurthub/pki/ca.crt) for yurthub
	if exist, err := util.FileExists(ycm.GetCaFile()); err != nil {
		return errors.Wrap(err, "couldn't stat ca.crt file")
	} else if !exist {
		cluster := kubeconfigutil.GetClusterFromKubeConfig(tlsBootstrapCfg)
		if cluster != nil {
			if err := certutil.WriteCert(ycm.GetCaFile(), cluster.CertificateAuthorityData); err != nil {
				return errors.Wrap(err, "couldn't save the CA certificate to disk")
			}
		} else {
			return errors.Errorf("couldn't prepare ca.crt(%s) file", ycm.GetCaFile())
		}
	} else {
		klog.V(2).Infof("%s file already exists, so reuse it", ycm.GetCaFile())
	}

	return nil
}

// Stop the cert manager loop
func (ycm *yurtHubCertManager) Stop() {
	ycm.apiServerClientCertManager.Stop()
	ycm.hubServerCertManager.Stop()
}

// Ready is used for checking client/server/ca certificates are prepared completely or not.
func (ycm *yurtHubCertManager) Ready() bool {
	var errs []error
	if ycm.GetHubServerCert() == nil {
		errs = append(errs, serverCertNotReadyError)
	}

	if ycm.GetAPIServerClientCert() == nil {
		errs = append(errs, apiServerClientCertNotReadyError)
	}

	if exist, _ := util.FileExists(ycm.GetCaFile()); !exist {
		errs = append(errs, caCertIsNotReadyError)
	}

	if len(errs) != 0 {
		klog.Errorf("hub certificates are not ready: %s", utilerrors.NewAggregate(errs).Error())
		return false
	}
	return true
}

// UpdateBootstrapConf is used for revising bootstrap conf file by new bearer token.
func (ycm *yurtHubCertManager) UpdateBootstrapConf(joinToken string) error {
	_, err := ycm.retrieveHubBootstrapConfig(joinToken)
	return err
}

// getPkiDir returns the directory for storing hub agent pki
func (ycm *yurtHubCertManager) getPkiDir() string {
	return filepath.Join(ycm.hubRunDir, hubPkiDirName)
}

// getBootstrapConfFile returns the path of yurthub bootstrap conf file
func (ycm *yurtHubCertManager) getBootstrapConfFile() string {
	return filepath.Join(ycm.hubRunDir, bootstrapConfigFileName)
}

// GetCaFile returns the path of ca file
func (ycm *yurtHubCertManager) GetCaFile() string {
	return filepath.Join(ycm.getPkiDir(), hubCaFileName)
}

// GetHubConfFile returns the path of yurtHub config file path
func (ycm *yurtHubCertManager) GetHubConfFile() string {
	return filepath.Join(ycm.hubRunDir, hubConfigFileName)
}

func (ycm *yurtHubCertManager) GetAPIServerClientCert() *tls.Certificate {
	return ycm.apiServerClientCertManager.Current()
}

func (ycm *yurtHubCertManager) GetHubServerCert() *tls.Certificate {
	return ycm.hubServerCertManager.Current()
}

func (ycm *yurtHubCertManager) GetHubServerCertFile() string {
	return ycm.serverCertStore.CurrentPath()
}

// newAPIServerClientCertificateManager create a certificate manager for yurthub component to prepare client certificate
// that used to proxy requests to remote kube-apiserver.
func (ycm *yurtHubCertManager) newAPIServerClientCertificateManager(fileStore certificate.FileStore, nodeName string, hubCertOrganizations []string) (certificate.Manager, error) {
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

func (ycm *yurtHubCertManager) generateCertClientFn(current *tls.Certificate) (clientset.Interface, error) {
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
	kubeconfig.Host = findActiveRemoteServer(ycm.remoteServers).String()
	// re-fix dial for conn management
	kubeconfig.Dial = ycm.dialer.DialContext

	// avoid tcp conn leak: certificate rotated, so close old tcp conn that used to rotate certificate
	klog.V(2).Infof("avoid tcp conn leak, close old tcp conn that used to rotate certificate")
	ycm.dialer.Close(strings.Trim(kubeconfig.Host, "https://"))

	return clientset.NewForConfig(kubeconfig)
}

// newHubServerCertificateManager create a certificate manager for yurthub component to prepare server certificate
// that used to handle requests from clients on edge nodes.
func (ycm *yurtHubCertManager) newHubServerCertificateManager(fileStore certificate.FileStore, nodeName string, certIPs []net.IP) (certificate.Manager, error) {
	kubeClientFn := func(current *tls.Certificate) (clientset.Interface, error) {
		// waiting for the certificate is generated
		_ = wait.PollInfinite(5*time.Second, func() (bool, error) {
			// keep polling until the yurthub client certificate is signed
			if ycm.apiServerClientCertManager.Current() != nil {
				return true, nil
			}
			klog.Infof("waiting for the controller-manager to sign the %s client certificate", ycm.hubName)
			return false, nil
		})

		if !yurtutil.IsNil(ycm.client) {
			return ycm.client, nil
		}

		return kubeconfigutil.ClientSetFromFile(ycm.GetHubConfFile())
	}

	// create a certificate manager for the yurthub server and run the csr approver for both yurthub
	return certfactory.NewCertManagerFactoryWithFnAndStore(kubeClientFn, fileStore).New(&certfactory.CertManagerConfig{
		ComponentName:  fmt.Sprintf("%s-server", ycm.hubName),
		SignerName:     certificatesv1.KubeletServingSignerName,
		ForServerUsage: true,
		CommonName:     fmt.Sprintf("system:node:%s", nodeName),
		Organizations:  []string{user.NodesGroup},
		IPs:            certIPs,
	})
}

func (ycm *yurtHubCertManager) retrieveHubBootstrapConfig(joinToken string) (*clientcmdapi.Config, error) {
	// retrieve bootstrap config info from cluster-info configmap by bootstrap token
	serverAddr := findActiveRemoteServer(ycm.remoteServers).Host
	if cfg, err := token.RetrieveValidatedConfigInfo(ycm.client, &token.BootstrapData{
		ServerAddr:   serverAddr,
		JoinToken:    joinToken,
		CaCertHashes: ycm.caCertHashes,
	}); err != nil {
		return nil, errors.Wrap(err, "couldn't retrieve bootstrap config info")
	} else {
		clusterInfo := kubeconfigutil.GetClusterFromKubeConfig(cfg)
		tlsBootstrapCfg := kubeconfigutil.CreateWithToken(
			fmt.Sprintf("https://%s", serverAddr),
			"kubernetes",
			"token-bootstrap-client",
			clusterInfo.CertificateAuthorityData,
			joinToken,
		)
		if err = kubeconfigutil.WriteToDisk(ycm.getBootstrapConfFile(), tlsBootstrapCfg); err != nil {
			return nil, errors.Wrap(err, "couldn't save bootstrap-hub.conf to disk")
		}

		return tlsBootstrapCfg, nil
	}
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

func findActiveRemoteServer(servers []*url.URL) *url.URL {
	if len(servers) == 0 {
		return nil
	} else if len(servers) == 1 {
		return servers[0]
	}

	for i := range servers {
		_, err := net.DialTimeout("tcp", servers[i].Host, 5*time.Second)
		if err == nil {
			return servers[i]
		}
	}

	return servers[0]
}
