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

package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	ipUtils "github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/network"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtcorev1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtclientset "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	yurtv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
)

// YurtHubConfiguration represents configuration of yurthub
type YurtHubConfiguration struct {
	LBMode                          string
	RemoteServers                   []*url.URL
	GCFrequency                     int
	NodeName                        string
	HeartbeatFailedRetry            int
	HeartbeatHealthyThreshold       int
	HeartbeatTimeoutSeconds         int
	HeartbeatIntervalSeconds        int
	MaxRequestInFlight              int
	EnableProfiling                 bool
	StorageWrapper                  cachemanager.StorageWrapper
	SerializerManager               *serializer.SerializerManager
	RESTMapperManager               *meta.RESTMapperManager
	SharedFactory                   informers.SharedInformerFactory
	YurtSharedFactory               yurtinformers.SharedInformerFactory
	WorkingMode                     util.WorkingMode
	KubeletHealthGracePeriod        time.Duration
	FilterManager                   *manager.Manager
	CoordinatorServer               *url.URL
	MinRequestTimeout               time.Duration
	TenantNs                        string
	NetworkMgr                      *network.NetworkManager
	CertManager                     certificate.YurtCertificateManager
	YurtHubServerServing            *apiserver.DeprecatedInsecureServingInfo
	YurtHubProxyServerServing       *apiserver.DeprecatedInsecureServingInfo
	YurtHubDummyProxyServerServing  *apiserver.DeprecatedInsecureServingInfo
	YurtHubSecureProxyServerServing *apiserver.SecureServingInfo
	YurtHubProxyServerAddr          string
	YurtHubNamespace                string
	ProxiedClient                   kubernetes.Interface
	DiskCachePath                   string
	CoordinatorPKIDir               string
	EnableCoordinator               bool
	CoordinatorServerURL            *url.URL
	CoordinatorStoragePrefix        string
	CoordinatorStorageAddr          string // ip:port
	CoordinatorClient               kubernetes.Interface
	LeaderElection                  componentbaseconfig.LeaderElectionConfiguration
}

// Complete converts *options.YurtHubOptions to *YurtHubConfiguration
func Complete(options *options.YurtHubOptions) (*YurtHubConfiguration, error) {
	us, err := parseRemoteServers(options.ServerAddr)
	if err != nil {
		return nil, err
	}

	var coordinatorServerURL *url.URL
	if options.EnableCoordinator {
		coordinatorServerURL, err = url.Parse(options.CoordinatorServerAddr)
		if err != nil {
			return nil, err
		}
	}

	storageManager, err := disk.NewDiskStorage(options.DiskCachePath)
	if err != nil {
		klog.Errorf("could not create storage manager, %v", err)
		return nil, err
	}
	storageWrapper := cachemanager.NewStorageWrapper(storageManager)
	serializerManager := serializer.NewSerializerManager()
	restMapperManager, err := meta.NewRESTMapperManager(options.DiskCachePath)
	if err != nil {
		klog.Errorf("failed to create restMapperManager at path %s, %v", options.DiskCachePath, err)
		return nil, err
	}

	workingMode := util.WorkingMode(options.WorkingMode)
	proxiedClient, sharedFactory, yurtSharedFactory, err := createClientAndSharedInformers(fmt.Sprintf("http://%s:%d", options.YurtHubProxyHost, options.YurtHubProxyPort), options.EnableNodePool)
	if err != nil {
		return nil, err
	}
	tenantNs := util.ParseTenantNsFromOrgs(options.YurtHubCertOrganizations)
	registerInformers(options, sharedFactory, yurtSharedFactory, workingMode, tenantNs)
	filterManager, err := manager.NewFilterManager(options, sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, us[0].Host)
	if err != nil {
		klog.Errorf("could not create filter manager, %v", err)
		return nil, err
	}

	cfg := &YurtHubConfiguration{
		LBMode:                    options.LBMode,
		RemoteServers:             us,
		GCFrequency:               options.GCFrequency,
		NodeName:                  options.NodeName,
		HeartbeatFailedRetry:      options.HeartbeatFailedRetry,
		HeartbeatHealthyThreshold: options.HeartbeatHealthyThreshold,
		HeartbeatTimeoutSeconds:   options.HeartbeatTimeoutSeconds,
		HeartbeatIntervalSeconds:  options.HeartbeatIntervalSeconds,
		MaxRequestInFlight:        options.MaxRequestInFlight,
		EnableProfiling:           options.EnableProfiling,
		WorkingMode:               workingMode,
		StorageWrapper:            storageWrapper,
		SerializerManager:         serializerManager,
		RESTMapperManager:         restMapperManager,
		SharedFactory:             sharedFactory,
		YurtSharedFactory:         yurtSharedFactory,
		KubeletHealthGracePeriod:  options.KubeletHealthGracePeriod,
		FilterManager:             filterManager,
		MinRequestTimeout:         options.MinRequestTimeout,
		TenantNs:                  tenantNs,
		YurtHubProxyServerAddr:    fmt.Sprintf("%s:%d", options.YurtHubProxyHost, options.YurtHubProxyPort),
		YurtHubNamespace:          options.YurtHubNamespace,
		ProxiedClient:             proxiedClient,
		DiskCachePath:             options.DiskCachePath,
		CoordinatorPKIDir:         filepath.Join(options.RootDir, "poolcoordinator"),
		EnableCoordinator:         options.EnableCoordinator,
		CoordinatorServerURL:      coordinatorServerURL,
		CoordinatorStoragePrefix:  options.CoordinatorStoragePrefix,
		CoordinatorStorageAddr:    options.CoordinatorStorageAddr,
		LeaderElection:            options.LeaderElection,
	}

	certMgr, err := createCertManager(options, us)
	if err != nil {
		return nil, err
	}
	cfg.CertManager = certMgr

	if options.EnableDummyIf {
		klog.V(2).Infof("create dummy network interface %s(%s) and init iptables manager", options.HubAgentDummyIfName, options.HubAgentDummyIfIP)
		networkMgr, err := network.NewNetworkManager(options)
		if err != nil {
			return nil, fmt.Errorf("could not create network manager, %w", err)
		}
		cfg.NetworkMgr = networkMgr
	}

	if err = prepareServerServing(options, certMgr, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseRemoteServers(serverAddr string) ([]*url.URL, error) {
	if serverAddr == "" {
		return make([]*url.URL, 0), fmt.Errorf("--server-addr should be set for hub agent")
	}
	servers := strings.Split(serverAddr, ",")
	us := make([]*url.URL, 0, len(servers))
	remoteServers := make([]string, 0, len(servers))
	for _, server := range servers {
		u, err := url.Parse(server)
		if err != nil {
			klog.Errorf("failed to parse server address %s, %v", servers, err)
			return us, err
		}
		if u.Scheme == "" {
			u.Scheme = "https"
		} else if u.Scheme != "https" {
			return us, fmt.Errorf("only https scheme is supported for server address(%s)", serverAddr)
		}
		us = append(us, u)
		remoteServers = append(remoteServers, u.String())
	}

	if len(us) < 1 {
		return us, fmt.Errorf("no server address is set, can not connect remote server")
	}
	klog.Infof("%s would connect remote servers: %s", projectinfo.GetHubName(), strings.Join(remoteServers, ","))

	return us, nil
}

// createSharedInformers create sharedInformers from the given proxyAddr.
func createClientAndSharedInformers(proxyAddr string, enableNodePool bool) (kubernetes.Interface, informers.SharedInformerFactory, yurtinformers.SharedInformerFactory, error) {
	var kubeConfig *rest.Config
	var yurtClient yurtclientset.Interface
	var err error
	kubeConfig, err = clientcmd.BuildConfigFromFlags(proxyAddr, "")
	if err != nil {
		return nil, nil, nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	fakeYurtClient := &fake.Clientset{}
	fakeWatch := watch.NewFake()
	fakeYurtClient.AddWatchReactor("nodepools", core.DefaultWatchReactor(fakeWatch, nil))
	// init yurtClient by fake client
	yurtClient = fakeYurtClient
	if enableNodePool {
		yurtClient, err = yurtclientset.NewForConfig(kubeConfig)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return client, informers.NewSharedInformerFactory(client, 24*time.Hour),
		yurtinformers.NewSharedInformerFactory(yurtClient, 24*time.Hour), nil
}

// registerInformers reconstruct node/nodePool/configmap informers
func registerInformers(options *options.YurtHubOptions,
	informerFactory informers.SharedInformerFactory,
	yurtInformerFactory yurtinformers.SharedInformerFactory,
	workingMode util.WorkingMode,
	tenantNs string) {
	// skip construct node/nodePool informers if service topology filter disabled
	serviceTopologyFilterEnabled := isServiceTopologyFilterEnabled(options)
	if serviceTopologyFilterEnabled {
		if workingMode == util.WorkingModeCloud {
			newNodeInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
				tweakListOptions := func(ops *metav1.ListOptions) {
					ops.FieldSelector = fields.Set{"metadata.name": options.NodeName}.String()
				}
				return coreinformers.NewFilteredNodeInformer(client, resyncPeriod, nil, tweakListOptions)
			}
			informerFactory.InformerFor(&corev1.Node{}, newNodeInformer)
		}

		if len(options.NodePoolName) != 0 {
			newNodePoolInformer := func(client yurtclientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
				tweakListOptions := func(ops *metav1.ListOptions) {
					ops.FieldSelector = fields.Set{"metadata.name": options.NodePoolName}.String()
				}
				return yurtv1alpha1.NewFilteredNodePoolInformer(client, resyncPeriod, nil, tweakListOptions)
			}

			yurtInformerFactory.InformerFor(&yurtcorev1alpha1.NodePool{}, newNodePoolInformer)
		}
	}

	newConfigmapInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.Set{"metadata.name": util.YurthubConfigMapName}.String()
		}
		return coreinformers.NewFilteredConfigMapInformer(client, options.YurtHubNamespace, resyncPeriod, nil, tweakListOptions)
	}
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigmapInformer)

	if tenantNs != "" {
		newSecretInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {

			return coreinformers.NewFilteredSecretInformer(client, tenantNs, resyncPeriod, nil, nil)
		}
		informerFactory.InformerFor(&corev1.Secret{}, newSecretInformer)
	}

}

// isServiceTopologyFilterEnabled is used to verify the service topology filter should be enabled or not.
func isServiceTopologyFilterEnabled(options *options.YurtHubOptions) bool {
	if !options.EnableResourceFilter {
		return false
	}

	for _, filterName := range options.DisabledResourceFilters {
		if filterName == filter.ServiceTopologyFilterName {
			return false
		}
	}

	if options.WorkingMode == string(util.WorkingModeCloud) {
		for i := range filter.DisabledInCloudMode {
			if filter.DisabledInCloudMode[i] == filter.ServiceTopologyFilterName {
				return false
			}
		}
	}

	return true
}

func createCertManager(options *options.YurtHubOptions, remoteServers []*url.URL) (certificate.YurtCertificateManager, error) {
	// use dummy ip and bind ip as cert IP SANs
	certIPs := ipUtils.RemoveDupIPs([]net.IP{
		net.ParseIP(options.HubAgentDummyIfIP),
		net.ParseIP(options.YurtHubHost),
		net.ParseIP(options.YurtHubProxyHost),
	})

	cfg := &token.CertificateManagerConfiguration{
		RootDir:                  options.RootDir,
		NodeName:                 options.NodeName,
		JoinToken:                options.JoinToken,
		BootstrapFile:            options.BootstrapFile,
		CaCertHashes:             options.CACertHashes,
		YurtHubCertOrganizations: options.YurtHubCertOrganizations,
		CertIPs:                  certIPs,
		RemoteServers:            remoteServers,
		Client:                   options.ClientForTest,
	}
	certManager, err := token.NewYurtHubCertManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create cert manager for yurthub, %v", err)
	}

	certManager.Start()
	err = wait.PollImmediate(5*time.Second, 4*time.Minute, func() (bool, error) {
		isReady := certManager.Ready()
		if isReady {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("hub certificates preparation failed, %v", err)
	}

	return certManager, nil
}

func prepareServerServing(options *options.YurtHubOptions, certMgr certificate.YurtCertificateManager, cfg *YurtHubConfiguration) error {
	if err := (&apiserveroptions.DeprecatedInsecureServingOptions{
		BindAddress: net.ParseIP(options.YurtHubHost),
		BindPort:    options.YurtHubPort,
		BindNetwork: "tcp",
	}).ApplyTo(&cfg.YurtHubServerServing); err != nil {
		return err
	}

	if err := (&apiserveroptions.DeprecatedInsecureServingOptions{
		BindAddress: net.ParseIP(options.YurtHubProxyHost),
		BindPort:    options.YurtHubProxyPort,
		BindNetwork: "tcp",
	}).ApplyTo(&cfg.YurtHubProxyServerServing); err != nil {
		return err
	}

	yurtHubSecureProxyHost := options.YurtHubProxyHost
	if options.EnableDummyIf {
		yurtHubSecureProxyHost = options.HubAgentDummyIfIP
		if err := (&apiserveroptions.DeprecatedInsecureServingOptions{
			BindAddress: net.ParseIP(options.HubAgentDummyIfIP),
			BindPort:    options.YurtHubProxyPort,
			BindNetwork: "tcp",
		}).ApplyTo(&cfg.YurtHubDummyProxyServerServing); err != nil {
			return err
		}
	}

	serverCertPath := certMgr.GetHubServerCertFile()
	serverCaPath := certMgr.GetCaFile()
	klog.V(2).Infof("server cert path is: %s, ca path is: %s", serverCertPath, serverCaPath)
	caBundleProvider, err := dynamiccertificates.NewDynamicCAContentFromFile("client-ca-bundle", serverCaPath)
	if err != nil {
		return err
	}

	if err := (&apiserveroptions.SecureServingOptions{
		BindAddress: net.ParseIP(yurtHubSecureProxyHost),
		BindPort:    options.YurtHubProxySecurePort,
		BindNetwork: "tcp",
		ServerCert: apiserveroptions.GeneratableKeyCert{
			CertKey: apiserveroptions.CertKey{
				CertFile: serverCertPath,
				KeyFile:  serverCertPath,
			},
		},
	}).ApplyTo(&cfg.YurtHubSecureProxyServerServing); err != nil {
		return err
	}
	cfg.YurtHubSecureProxyServerServing.ClientCA = caBundleProvider
	cfg.YurtHubSecureProxyServerServing.DisableHTTP2 = true

	return nil
}
