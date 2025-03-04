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
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	pkgutil "github.com/openyurtio/openyurt/pkg/util"
	utiloptions "github.com/openyurtio/openyurt/pkg/util/kubernetes/apiserver/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	certificatemgr "github.com/openyurtio/openyurt/pkg/yurthub/certificate/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/network"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// YurtHubConfiguration represents configuration of yurthub
type YurtHubConfiguration struct {
	LBMode                          string
	RemoteServers                   []*url.URL
	TenantKasService                string // ip:port, used in local mode
	GCFrequency                     int
	NodeName                        string
	HeartbeatFailedRetry            int
	HeartbeatHealthyThreshold       int
	HeartbeatTimeoutSeconds         int
	HeartbeatIntervalSeconds        int
	EnableProfiling                 bool
	StorageWrapper                  cachemanager.StorageWrapper
	SerializerManager               *serializer.SerializerManager
	RESTMapperManager               *meta.RESTMapperManager
	SharedFactory                   informers.SharedInformerFactory
	DynamicSharedFactory            dynamicinformer.DynamicSharedInformerFactory
	WorkingMode                     util.WorkingMode
	KubeletHealthGracePeriod        time.Duration
	FilterFinder                    filter.FilterFinder
	MinRequestTimeout               time.Duration
	NetworkMgr                      *network.NetworkManager
	CertManager                     certificate.YurtCertificateManager
	YurtHubServerServing            *apiserver.DeprecatedInsecureServingInfo
	YurtHubProxyServerServing       *apiserver.DeprecatedInsecureServingInfo
	YurtHubDummyProxyServerServing  *apiserver.DeprecatedInsecureServingInfo
	YurtHubSecureProxyServerServing *apiserver.SecureServingInfo
	YurtHubMultiplexerServerServing *apiserver.SecureServingInfo
	DiskCachePath                   string
	ConfigManager                   *configuration.Manager
	TenantManager                   tenant.Interface
	TransportAndDirectClientManager transport.Interface
	LoadBalancerForLeaderHub        remote.LoadBalancer
	PoolScopeResources              []schema.GroupVersionResource
	PortForMultiplexer              int
	NodePoolName                    string
}

// Complete converts *options.YurtHubOptions to *YurtHubConfiguration
func Complete(options *options.YurtHubOptions, stopCh <-chan struct{}) (*YurtHubConfiguration, error) {
	cfg := &YurtHubConfiguration{
		NodeName:        options.NodeName,
		WorkingMode:     util.WorkingMode(options.WorkingMode),
		EnableProfiling: options.EnableProfiling,
	}

	switch cfg.WorkingMode {
	case util.WorkingModeLocal:
		// if yurthub is in local mode, cfg.TenantKasService is used to represented as the service address (ip:port) of multiple apiserver daemonsets
		cfg.TenantKasService = options.ServerAddr
		_, sharedFactory, _, err := createClientAndSharedInformerFactories(options.HostControlPlaneAddr, "")
		if err != nil {
			return nil, err
		}

		// list/watch endpoints from host cluster in order to resolve tenant cluster address.
		newEndpointsInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
			informer := coreinformers.NewFilteredEndpointsInformer(client, "kube-public", resyncPeriod, nil, nil)
			informer.SetTransform(pkgutil.TransformStripManagedFields())
			return informer
		}
		sharedFactory.InformerFor(&corev1.Endpoints{}, newEndpointsInformer)
		cfg.SharedFactory = sharedFactory
	case util.WorkingModeCloud, util.WorkingModeEdge:
		us, err := parseRemoteServers(options.ServerAddr)
		if err != nil {
			return nil, err
		}
		cfg.RemoteServers = us
		cfg.LBMode = options.LBMode
		tenantNamespce := util.ParseTenantNsFromOrgs(options.YurtHubCertOrganizations)
		cfg.PoolScopeResources = options.PoolScopeResources
		cfg.PortForMultiplexer = options.PortForMultiplexer
		cfg.NodePoolName = options.NodePoolName

		// prepare some basic configurations as following:
		// - serializer manager: used for managing serializer for encoding or decoding response from kube-apiserver.
		// - restMapper manager: used for recording mappings between GVK and GVR, and CRD information.
		// - sharedInformers/dynamicSharedInformers: used for list/watch resources(such as yurt-hub-cfg configmap, nodebucket, etc.) which needed by yurthub itself.
		// - certificate manager: used for managing client certificate(accessing kube-apiserver) and server certificate(provide serving on the node).
		// - transport manager: used for managing the underlay transport which connecting to cloud kube-apiserver.
		restMapperManager, err := meta.NewRESTMapperManager(options.DiskCachePath)
		if err != nil {
			klog.Errorf("could not create restMapperManager at path %s, %v", options.DiskCachePath, err)
			return nil, err
		}

		proxiedClient, sharedFactory, dynamicSharedFactory, err := createClientAndSharedInformerFactories(fmt.Sprintf("%s:%d", options.YurtHubProxyHost, options.YurtHubProxyPort), options.NodePoolName)
		if err != nil {
			return nil, err
		}
		registerInformers(sharedFactory, options.YurtHubNamespace, options.NodePoolName, options.NodeName, (cfg.WorkingMode == util.WorkingModeCloud), tenantNamespce)

		certMgr, err := certificatemgr.NewYurtHubCertManager(options, us)
		if err != nil {
			return nil, err
		}
		certMgr.Start()
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 4*time.Minute, true, func(ctx context.Context) (bool, error) {
			isReady := certMgr.Ready()
			if isReady {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return nil, fmt.Errorf("hub certificates preparation failed, %v", err)
		}

		transportAndClientManager, err := transport.NewTransportAndClientManager(us, options.HeartbeatTimeoutSeconds, certMgr, stopCh)
		if err != nil {
			return nil, fmt.Errorf("could not new transport manager, %w", err)
		}

		cfg.SerializerManager = serializer.NewSerializerManager()
		cfg.RESTMapperManager = restMapperManager
		cfg.SharedFactory = sharedFactory
		cfg.DynamicSharedFactory = dynamicSharedFactory
		cfg.CertManager = certMgr
		cfg.TransportAndDirectClientManager = transportAndClientManager
		cfg.TenantManager = tenant.New(tenantNamespce, sharedFactory, stopCh)

		// create feature configurations for both cloud and edge working mode as following:
		// - configuration manager: monitor yurt-hub-cfg configmap and adopting changes dynamically.
		// - filter finder: filter response from kube-apiserver according to request.
		// - multiplexer: aggregating requests for pool scope metadata in order to reduce overhead of cloud kube-apiserver
		// - network manager: ensuring a dummy interface in order to serve tls requests on the node.
		// - others: prepare server servings.
		configManager := configuration.NewConfigurationManager(options.NodeName, sharedFactory)
		filterFinder, err := manager.NewFilterManager(options, sharedFactory, dynamicSharedFactory, proxiedClient, cfg.SerializerManager, configManager)
		if err != nil {
			klog.Errorf("could not create filter manager, %v", err)
			return nil, err
		}

		cfg.ConfigManager = configManager
		cfg.FilterFinder = filterFinder

		if options.EnableDummyIf {
			klog.V(2).Infof("create dummy network interface %s(%s)", options.HubAgentDummyIfName, options.HubAgentDummyIfIP)
			networkMgr, err := network.NewNetworkManager(options)
			if err != nil {
				return nil, fmt.Errorf("could not create network manager, %w", err)
			}
			cfg.NetworkMgr = networkMgr
		}

		if err = prepareServerServing(options, certMgr, cfg); err != nil {
			return nil, err
		}

		// following parameter is only used on edge working mode
		cfg.DiskCachePath = options.DiskCachePath
		cfg.GCFrequency = options.GCFrequency
		cfg.HeartbeatFailedRetry = options.HeartbeatFailedRetry
		cfg.HeartbeatHealthyThreshold = options.HeartbeatHealthyThreshold
		cfg.HeartbeatTimeoutSeconds = options.HeartbeatTimeoutSeconds
		cfg.HeartbeatIntervalSeconds = options.HeartbeatIntervalSeconds
		cfg.KubeletHealthGracePeriod = options.KubeletHealthGracePeriod
		cfg.MinRequestTimeout = options.MinRequestTimeout
	default:
		return nil, fmt.Errorf("unsupported working mode(%s)", options.WorkingMode)
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
			klog.Errorf("could not parse server address %q, %v", server, err)
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

// createClientAndSharedInformerFactories create client and sharedInformers from the given proxyAddr.
func createClientAndSharedInformerFactories(serverAddr, nodePoolName string) (kubernetes.Interface, informers.SharedInformerFactory, dynamicinformer.DynamicSharedInformerFactory, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(fmt.Sprintf("http://%s", serverAddr), "")
	if err != nil {
		return nil, nil, nil, err
	}
	kubeConfig.UserAgent = projectinfo.ShortHubVersion()

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 24*time.Hour)
	if len(nodePoolName) != 0 {
		dynamicInformerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 24*time.Hour, metav1.NamespaceAll, func(opts *metav1.ListOptions) {
			opts.LabelSelector = labels.Set{initializer.LabelNodePoolName: nodePoolName}.String()
		})
	}

	return client, informers.NewSharedInformerFactory(client, 24*time.Hour), dynamicInformerFactory, nil
}

// registerInformers reconstruct configmap/secret/pod/service informers on cloud and edge working mode.
func registerInformers(
	informerFactory informers.SharedInformerFactory,
	namespace string,
	poolName string,
	nodeName string,
	enablePodInformer bool,
	tenantNs string) {

	// configmap informer is used for list/watching yurt-hub-cfg configmap and leader-hub-{poolName} configmap.
	// yurt-hub-cfg configmap includes configurations about cache agents and filters which are needed by approver in filter and cache manager on cloud and edge working mode.
	// leader-hub-{nodePoolName} configmap includes leader election configurations which are used by multiplexer manager.
	newConfigmapInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("openyurt.io/configmap-name in (%s, %s)", util.YurthubConfigMapName, "leader-hub-"+poolName)
		}
		informer := coreinformers.NewFilteredConfigMapInformer(client, namespace, resyncPeriod, nil, tweakListOptions)
		informer.SetTransform(pkgutil.TransformStripManagedFields())
		return informer
	}
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigmapInformer)

	// secret informer is used by Tenant manager, this feature is not enabled in general.
	if tenantNs != "" {
		newSecretInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
			informer := coreinformers.NewFilteredSecretInformer(client, tenantNs, resyncPeriod, nil, nil)
			informer.SetTransform(pkgutil.TransformStripManagedFields())
			return informer
		}
		informerFactory.InformerFor(&corev1.Secret{}, newSecretInformer)
	}

	// pod informer is used for list/watching pods of specified node and used by OTA updater on cloud working mode.
	if enablePodInformer {
		newPodInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
			listOptions := func(ops *metav1.ListOptions) {
				ops.FieldSelector = fields.Set{"spec.nodeName": nodeName}.String()
			}
			informer := coreinformers.NewFilteredPodInformer(client, "", resyncPeriod, nil, listOptions)
			informer.SetTransform(pkgutil.TransformStripManagedFields())
			return informer
		}
		informerFactory.InformerFor(&corev1.Pod{}, newPodInformer)
	}

	// service informer is used for list/watch all services in the cluster, and used by serviceTopology Filter on cloud and edge mode.
	newServiceInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		informer := coreinformers.NewFilteredServiceInformer(client, "", resyncPeriod, nil, nil)
		informer.SetTransform(pkgutil.TransformStripManagedFields())
		return informer
	}
	informerFactory.InformerFor(&corev1.Service{}, newServiceInformer)
}

func prepareServerServing(options *options.YurtHubOptions, certMgr certificate.YurtCertificateManager, cfg *YurtHubConfiguration) error {
	if err := (&utiloptions.InsecureServingOptions{
		BindAddress: net.ParseIP(options.YurtHubHost),
		BindPort:    options.YurtHubPort,
		BindNetwork: "tcp",
	}).ApplyTo(&cfg.YurtHubServerServing); err != nil {
		return err
	}

	if err := (&utiloptions.InsecureServingOptions{
		BindAddress: net.ParseIP(options.YurtHubProxyHost),
		BindPort:    options.YurtHubProxyPort,
		BindNetwork: "tcp",
	}).ApplyTo(&cfg.YurtHubProxyServerServing); err != nil {
		return err
	}

	yurtHubSecureProxyHost := options.YurtHubProxyHost
	if options.EnableDummyIf {
		yurtHubSecureProxyHost = options.HubAgentDummyIfIP
		if err := (&utiloptions.InsecureServingOptions{
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

	if err := (&apiserveroptions.SecureServingOptions{
		BindAddress: net.ParseIP(options.NodeIP),
		BindPort:    options.PortForMultiplexer,
		BindNetwork: "tcp",
		ServerCert: apiserveroptions.GeneratableKeyCert{
			CertKey: apiserveroptions.CertKey{
				CertFile: serverCertPath,
				KeyFile:  serverCertPath,
			},
		},
	}).ApplyTo(&cfg.YurtHubMultiplexerServerServing); err != nil {
		return err
	}
	cfg.YurtHubMultiplexerServerServing.ClientCA = caBundleProvider
	cfg.YurtHubMultiplexerServerServing.DisableHTTP2 = true
	return nil
}

func ReadinessCheck(cfg *YurtHubConfiguration) error {
	if cfg.CertManager != nil {
		if ready := cfg.CertManager.Ready(); !ready {
			return fmt.Errorf("certificates are not ready")
		}
	}

	if cfg.ConfigManager != nil {
		if ready := cfg.ConfigManager.HasSynced(); !ready {
			return fmt.Errorf("yurt-hub-cfg configmap is not synced")
		}
	}

	if cfg.FilterFinder != nil {
		if synced := cfg.FilterFinder.HasSynced(); !synced {
			return fmt.Errorf("resources needed by filters are not synced")
		}
	}

	return nil
}
