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
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
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
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
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
	LBMode                            string
	RemoteServers                     []*url.URL
	YurtHubServerAddr                 string
	YurtHubCertOrganizations          []string
	YurtHubProxyServerAddr            string
	YurtHubProxyServerSecureAddr      string
	YurtHubProxyServerDummyAddr       string
	YurtHubProxyServerSecureDummyAddr string
	DiskCachePath                     string
	GCFrequency                       int
	CertMgrMode                       string
	KubeletRootCAFilePath             string
	KubeletPairFilePath               string
	NodeName                          string
	HeartbeatFailedRetry              int
	HeartbeatHealthyThreshold         int
	HeartbeatTimeoutSeconds           int
	HeartbeatIntervalSeconds          int
	MaxRequestInFlight                int
	JoinToken                         string
	RootDir                           string
	EnableProfiling                   bool
	EnableDummyIf                     bool
	EnableIptables                    bool
	HubAgentDummyIfName               string
	StorageWrapper                    cachemanager.StorageWrapper
	SerializerManager                 *serializer.SerializerManager
	RESTMapperManager                 *meta.RESTMapperManager
	TLSConfig                         *tls.Config
	SharedFactory                     informers.SharedInformerFactory
	YurtSharedFactory                 yurtinformers.SharedInformerFactory
	WorkingMode                       util.WorkingMode
	KubeletHealthGracePeriod          time.Duration
	FilterManager                     *manager.Manager
	CertIPs                           []net.IP
	CoordinatorServer                 *url.URL
	MinRequestTimeout                 time.Duration
	CoordinatorStoragePrefix          string
	CoordinatorStorageAddr            string // ip:port
	CoordinatorStorageCaFile          string
	CoordinatorStorageCertFile        string
	CoordinatorStorageKeyFile         string
	LeaderElection                    componentbaseconfig.LeaderElectionConfiguration
	CoordinatorClient                 kubernetes.Interface
}

// Complete converts *options.YurtHubOptions to *YurtHubConfiguration
func Complete(options *options.YurtHubOptions) (*YurtHubConfiguration, error) {
	us, err := parseRemoteServers(options.ServerAddr)
	if err != nil {
		return nil, err
	}

	hubCertOrgs := make([]string, 0)
	if options.YurtHubCertOrganizations != "" {
		for _, orgStr := range strings.Split(options.YurtHubCertOrganizations, ",") {
			hubCertOrgs = append(hubCertOrgs, orgStr)
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

	hubServerAddr := net.JoinHostPort(options.YurtHubHost, options.YurtHubPort)
	proxyServerAddr := net.JoinHostPort(options.YurtHubProxyHost, options.YurtHubProxyPort)
	proxySecureServerAddr := net.JoinHostPort(options.YurtHubProxyHost, options.YurtHubProxySecurePort)
	proxyServerDummyAddr := net.JoinHostPort(options.HubAgentDummyIfIP, options.YurtHubProxyPort)
	proxySecureServerDummyAddr := net.JoinHostPort(options.HubAgentDummyIfIP, options.YurtHubProxySecurePort)
	workingMode := util.WorkingMode(options.WorkingMode)
	proxiedClient, err := buildProxiedClient(fmt.Sprintf("http://%s", proxyServerAddr))
	if err != nil {
		return nil, err
	}
	sharedFactory := informers.NewSharedInformerFactory(proxiedClient, 24*time.Hour)
	yurtSharedFactory, err := createYurtSharedInformers(proxiedClient, options.EnableNodePool)
	if err != nil {
		return nil, err
	}
	tenantNs := util.ParseTenantNs(options.YurtHubCertOrganizations)
	registerInformers(sharedFactory, yurtSharedFactory, workingMode, serviceTopologyFilterEnabled(options), options.NodePoolName, options.NodeName, tenantNs)
	filterManager, err := manager.NewFilterManager(options, sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, us[0].Host)
	if err != nil {
		klog.Errorf("could not create filter manager, %v", err)
		return nil, err
	}

	// use dummy ip and bind ip as cert IP SANs
	certIPs := ipUtils.RemoveDupIPs([]net.IP{
		net.ParseIP(options.HubAgentDummyIfIP),
		net.ParseIP(options.YurtHubHost),
		net.ParseIP(options.YurtHubProxyHost),
	})

	cfg := &YurtHubConfiguration{
		LBMode:                            options.LBMode,
		RemoteServers:                     us,
		YurtHubServerAddr:                 hubServerAddr,
		YurtHubCertOrganizations:          hubCertOrgs,
		YurtHubProxyServerAddr:            proxyServerAddr,
		YurtHubProxyServerSecureAddr:      proxySecureServerAddr,
		YurtHubProxyServerDummyAddr:       proxyServerDummyAddr,
		YurtHubProxyServerSecureDummyAddr: proxySecureServerDummyAddr,
		DiskCachePath:                     options.DiskCachePath,
		GCFrequency:                       options.GCFrequency,
		KubeletRootCAFilePath:             options.KubeletRootCAFilePath,
		KubeletPairFilePath:               options.KubeletPairFilePath,
		NodeName:                          options.NodeName,
		HeartbeatFailedRetry:              options.HeartbeatFailedRetry,
		HeartbeatHealthyThreshold:         options.HeartbeatHealthyThreshold,
		HeartbeatTimeoutSeconds:           options.HeartbeatTimeoutSeconds,
		HeartbeatIntervalSeconds:          options.HeartbeatIntervalSeconds,
		MaxRequestInFlight:                options.MaxRequestInFlight,
		JoinToken:                         options.JoinToken,
		RootDir:                           options.RootDir,
		EnableProfiling:                   options.EnableProfiling,
		EnableDummyIf:                     options.EnableDummyIf,
		EnableIptables:                    options.EnableIptables,
		HubAgentDummyIfName:               options.HubAgentDummyIfName,
		WorkingMode:                       workingMode,
		StorageWrapper:                    storageWrapper,
		SerializerManager:                 serializerManager,
		RESTMapperManager:                 restMapperManager,
		SharedFactory:                     sharedFactory,
		YurtSharedFactory:                 yurtSharedFactory,
		KubeletHealthGracePeriod:          options.KubeletHealthGracePeriod,
		FilterManager:                     filterManager,
		CertIPs:                           certIPs,
		MinRequestTimeout:                 options.MinRequestTimeout,
		CoordinatorStoragePrefix:          options.CoordinatorStoragePrefix,
		CoordinatorStorageAddr:            options.CoordinatorStorageAddr,
		CoordinatorStorageCaFile:          options.CoordinatorStorageCaFile,
		CoordinatorStorageCertFile:        options.CoordinatorStorageCertFile,
		CoordinatorStorageKeyFile:         options.CoordinatorStorageKeyFile,
		LeaderElection:                    options.LeaderElection,
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

func buildProxiedClient(proxyAddr string) (kubernetes.Interface, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(proxyAddr, "")
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// createSharedInformers create sharedInformers from the given proxyAddr.
func createYurtSharedInformers(proxiedClient kubernetes.Interface, enableNodePool bool) (yurtinformers.SharedInformerFactory, error) {
	var kubeConfig *rest.Config
	var yurtClient yurtclientset.Interface
	var err error

	fakeYurtClient := &fake.Clientset{}
	fakeWatch := watch.NewFake()
	fakeYurtClient.AddWatchReactor("nodepools", core.DefaultWatchReactor(fakeWatch, nil))
	// init yurtClient by fake client
	yurtClient = fakeYurtClient
	if enableNodePool {
		yurtClient, err = yurtclientset.NewForConfig(kubeConfig)
		if err != nil {
			return nil, err
		}
	}

	return yurtinformers.NewSharedInformerFactory(yurtClient, 24*time.Hour), nil
}

// registerInformers reconstruct node/nodePool/configmap informers
func registerInformers(informerFactory informers.SharedInformerFactory,
	yurtInformerFactory yurtinformers.SharedInformerFactory,
	workingMode util.WorkingMode,
	serviceTopologyFilterEnabled bool,
	nodePoolName, nodeName string,
	tenantNs string) {
	// skip construct node/nodePool informers if service topology filter disabled
	if serviceTopologyFilterEnabled {
		if workingMode == util.WorkingModeCloud {
			newNodeInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
				tweakListOptions := func(options *metav1.ListOptions) {
					options.FieldSelector = fields.Set{"metadata.name": nodeName}.String()
				}
				return coreinformers.NewFilteredNodeInformer(client, resyncPeriod, nil, tweakListOptions)
			}
			informerFactory.InformerFor(&corev1.Node{}, newNodeInformer)
		}

		if len(nodePoolName) != 0 {
			newNodePoolInformer := func(client yurtclientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
				tweakListOptions := func(options *metav1.ListOptions) {
					options.FieldSelector = fields.Set{"metadata.name": nodePoolName}.String()
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
		return coreinformers.NewFilteredConfigMapInformer(client, util.YurtHubNamespace, resyncPeriod, nil, tweakListOptions)
	}
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigmapInformer)

	if tenantNs != "" {
		newSecretInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {

			return coreinformers.NewFilteredSecretInformer(client, tenantNs, resyncPeriod, nil, nil)
		}
		informerFactory.InformerFor(&corev1.Secret{}, newSecretInformer)
	}

}

// serviceTopologyFilterEnabled is used to verify the service topology filter should be enabled or not.
func serviceTopologyFilterEnabled(options *options.YurtHubOptions) bool {
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
