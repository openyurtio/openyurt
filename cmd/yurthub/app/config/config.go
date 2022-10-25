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

	componentbaseconfig "k8s.io/component-base/config"

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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	ipUtils "github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/factory"
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
	FilterManager                     *filter.Manager
	CertIPs                           []net.IP
	CoordinatorServer                 *url.URL
	LeaderElection                    componentbaseconfig.LeaderElectionConfiguration
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

	storageManager, err := factory.CreateStorage(options.DiskCachePath)
	if err != nil {
		klog.Errorf("could not create storage manager, %v", err)
		return nil, err
	}
	storageWrapper := cachemanager.NewStorageWrapper(storageManager)
	serializerManager := serializer.NewSerializerManager()
	restMapperManager := meta.NewRESTMapperManager(storageManager)

	hubServerAddr := net.JoinHostPort(options.YurtHubHost, options.YurtHubPort)
	proxyServerAddr := net.JoinHostPort(options.YurtHubProxyHost, options.YurtHubProxyPort)
	proxySecureServerAddr := net.JoinHostPort(options.YurtHubProxyHost, options.YurtHubProxySecurePort)
	proxyServerDummyAddr := net.JoinHostPort(options.HubAgentDummyIfIP, options.YurtHubProxyPort)
	proxySecureServerDummyAddr := net.JoinHostPort(options.HubAgentDummyIfIP, options.YurtHubProxySecurePort)
	workingMode := util.WorkingMode(options.WorkingMode)
	sharedFactory, yurtSharedFactory, err := createSharedInformers(fmt.Sprintf("http://%s", proxyServerAddr), options.EnableNodePool)
	if err != nil {
		return nil, err
	}
	tenantNs := util.ParseTenantNs(options.YurtHubCertOrganizations)
	registerInformers(sharedFactory, yurtSharedFactory, workingMode, serviceTopologyFilterEnabled(options), options.NodePoolName, options.NodeName, tenantNs)
	filterManager, err := createFilterManager(options, sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, us[0].Host, proxySecureServerDummyAddr, proxySecureServerAddr)

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

// createSharedInformers create sharedInformers from the given proxyAddr.
func createSharedInformers(proxyAddr string, enableNodePool bool) (informers.SharedInformerFactory, yurtinformers.SharedInformerFactory, error) {
	var kubeConfig *rest.Config
	var yurtClient yurtclientset.Interface
	var err error
	kubeConfig, err = clientcmd.BuildConfigFromFlags(proxyAddr, "")
	if err != nil {
		return nil, nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, err
	}

	fakeYurtClient := &fake.Clientset{}
	fakeWatch := watch.NewFake()
	fakeYurtClient.AddWatchReactor("nodepools", core.DefaultWatchReactor(fakeWatch, nil))
	// init yurtClient by fake client
	yurtClient = fakeYurtClient
	if enableNodePool {
		yurtClient, err = yurtclientset.NewForConfig(kubeConfig)
		if err != nil {
			return nil, nil, err
		}
	}

	return informers.NewSharedInformerFactory(client, 24*time.Hour),
		yurtinformers.NewSharedInformerFactory(yurtClient, 24*time.Hour), nil
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

// registerAllFilters by order, the front registered filter will be
// called before the behind registered ones.
func registerAllFilters(filters *filter.Filters) {
	servicetopology.Register(filters)
	masterservice.Register(filters)
	discardcloudservice.Register(filters)
}

// generateNameToFilterMapping return union filters that initializations completed.
func generateNameToFilterMapping(filters *filter.Filters,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	serializerManager *serializer.SerializerManager,
	storageWrapper cachemanager.StorageWrapper,
	workingMode util.WorkingMode,
	nodeName, mutatedMasterServiceAddr string) (map[string]filter.Runner, error) {
	if filters == nil {
		return nil, nil
	}

	genericInitializer := initializer.New(sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, nodeName, mutatedMasterServiceAddr, workingMode)
	initializerChain := filter.FilterInitializers{}
	initializerChain = append(initializerChain, genericInitializer)
	return filters.NewFromFilters(initializerChain)
}

// createFilterManager will create a filter manager for data filtering framework.
func createFilterManager(options *options.YurtHubOptions,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	serializerManager *serializer.SerializerManager,
	storageWrapper cachemanager.StorageWrapper,
	apiserverAddr string,
	proxySecureServerDummyAddr string,
	proxySecureServerAddr string,
) (*filter.Manager, error) {
	if !options.EnableResourceFilter {
		return nil, nil
	}

	if options.WorkingMode == string(util.WorkingModeCloud) {
		options.DisabledResourceFilters = append(options.DisabledResourceFilters, filter.DisabledInCloudMode...)
	}
	filters := filter.NewFilters(options.DisabledResourceFilters)
	registerAllFilters(filters)

	mutatedMasterServiceAddr := apiserverAddr
	if options.AccessServerThroughHub {
		if options.EnableDummyIf {
			mutatedMasterServiceAddr = proxySecureServerDummyAddr
		} else {
			mutatedMasterServiceAddr = proxySecureServerAddr
		}
	}

	filterMapping, err := generateNameToFilterMapping(filters, sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, util.WorkingMode(options.WorkingMode), options.NodeName, mutatedMasterServiceAddr)
	if err != nil {
		return nil, err
	}

	return filter.NewFilterManager(sharedFactory, filterMapping), nil
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
