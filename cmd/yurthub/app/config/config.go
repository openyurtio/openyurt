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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	certificatemgr "github.com/openyurtio/openyurt/pkg/yurthub/certificate/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/network"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
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
	NodePoolInformerFactory         dynamicinformer.DynamicSharedInformerFactory
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
		klog.Errorf("could not create restMapperManager at path %s, %v", options.DiskCachePath, err)
		return nil, err
	}

	workingMode := util.WorkingMode(options.WorkingMode)
	proxiedClient, sharedFactory, dynamicSharedFactory, err := createClientAndSharedInformers(options)
	if err != nil {
		return nil, err
	}
	tenantNs := util.ParseTenantNsFromOrgs(options.YurtHubCertOrganizations)
	registerInformers(options, sharedFactory, workingMode, tenantNs)
	filterManager, err := manager.NewFilterManager(options, sharedFactory, dynamicSharedFactory, proxiedClient, serializerManager, us[0].Host)
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
		NodePoolInformerFactory:   dynamicSharedFactory,
		KubeletHealthGracePeriod:  options.KubeletHealthGracePeriod,
		FilterManager:             filterManager,
		MinRequestTimeout:         options.MinRequestTimeout,
		TenantNs:                  tenantNs,
		YurtHubProxyServerAddr:    fmt.Sprintf("%s:%d", options.YurtHubProxyHost, options.YurtHubProxyPort),
		YurtHubNamespace:          options.YurtHubNamespace,
		ProxiedClient:             proxiedClient,
		DiskCachePath:             options.DiskCachePath,
		CoordinatorPKIDir:         filepath.Join(options.RootDir, "yurtcoordinator"),
		EnableCoordinator:         options.EnableCoordinator,
		CoordinatorServerURL:      coordinatorServerURL,
		CoordinatorStoragePrefix:  options.CoordinatorStoragePrefix,
		CoordinatorStorageAddr:    options.CoordinatorStorageAddr,
		LeaderElection:            options.LeaderElection,
	}

	certMgr, err := certificatemgr.NewYurtHubCertManager(options, us)
	if err != nil {
		return nil, err
	}
	certMgr.Start()
	err = wait.PollImmediate(5*time.Second, 4*time.Minute, func() (bool, error) {
		isReady := certMgr.Ready()
		if isReady {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("hub certificates preparation failed, %v", err)
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

// createClientAndSharedInformers create kubeclient and sharedInformers from the given proxyAddr.
func createClientAndSharedInformers(options *options.YurtHubOptions) (kubernetes.Interface, informers.SharedInformerFactory, dynamicinformer.DynamicSharedInformerFactory, error) {
	var kubeConfig *rest.Config
	var err error
	kubeConfig, err = clientcmd.BuildConfigFromFlags(fmt.Sprintf("http://%s:%d", options.YurtHubProxyHost, options.YurtHubProxyPort), "")
	if err != nil {
		return nil, nil, nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 24*time.Hour)
	if len(options.NodePoolName) != 0 {
		if options.EnablePoolServiceTopology {
			dynamicInformerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 24*time.Hour, metav1.NamespaceAll, func(opts *metav1.ListOptions) {
				opts.LabelSelector = labels.Set{initializer.LabelNodePoolName: options.NodePoolName}.String()
			})
		} else if options.EnableNodePool {
			dynamicInformerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 24*time.Hour, metav1.NamespaceAll, func(opts *metav1.ListOptions) {
				opts.FieldSelector = fields.Set{"metadata.name": options.NodePoolName}.String()
			})
		}
	}

	return client, informers.NewSharedInformerFactory(client, 24*time.Hour), dynamicInformerFactory, nil
}

// registerInformers reconstruct configmap/secret/pod informers
func registerInformers(options *options.YurtHubOptions,
	informerFactory informers.SharedInformerFactory,
	workingMode util.WorkingMode,
	tenantNs string) {
	// configmap informer is used by Yurthub filter approver
	newConfigmapInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.Set{"metadata.name": util.YurthubConfigMapName}.String()
		}
		return coreinformers.NewFilteredConfigMapInformer(client, options.YurtHubNamespace, resyncPeriod, nil, tweakListOptions)
	}
	informerFactory.InformerFor(&corev1.ConfigMap{}, newConfigmapInformer)

	// secret informer is used by Tenant manager, this feature is not enabled in general.
	if tenantNs != "" {
		newSecretInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
			return coreinformers.NewFilteredSecretInformer(client, tenantNs, resyncPeriod, nil, nil)
		}
		informerFactory.InformerFor(&corev1.Secret{}, newSecretInformer)
	}

	// pod informer is used by OTA updater on cloud working mode
	if workingMode == util.WorkingModeCloud {
		newPodInformer := func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
			listOptions := func(ops *metav1.ListOptions) {
				ops.FieldSelector = fields.Set{"spec.nodeName": options.NodeName}.String()
			}
			return coreinformers.NewFilteredPodInformer(client, "", resyncPeriod, nil, listOptions)
		}
		informerFactory.InformerFor(&corev1.Pod{}, newPodInformer)
	}
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
