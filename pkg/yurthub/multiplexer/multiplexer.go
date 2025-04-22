/*
Copyright 2024 The OpenYurt Authors.

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

package multiplexer

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	ystorage "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	PoolScopeMetadataKey = "pool-scoped-metadata"
	LeaderEndpointsKey   = "leaders"
	EnableLeaderElection = "enable-leader-election"

	PoolSourceForPoolScopeMetadata      = "pool"
	APIServerSourceForPoolScopeMetadata = "api"
)

type MultiplexerManager struct {
	filterStoreManager      *filterStoreManager
	healthCheckerForLeaders healthchecker.Interface
	loadBalancerForLeaders  remote.Server
	portForLeaderHub        int
	nodeName                string
	multiplexerUserAgent    string

	sync.RWMutex
	lazyLoadedGVRCache            map[string]Interface
	lazyLoadedGVRCacheDestroyFunc map[string]func()
	sourceForPoolScopeMetadata    string
	poolScopeMetadata             sets.Set[string]
	leaderAddresses               sets.Set[string]
	configMapSynced               cache.InformerSynced
}

func NewRequestMultiplexerManager(
	cfg *config.YurtHubConfiguration,
	storageProvider ystorage.StorageProvider,
	healthCheckerForLeaders healthchecker.Interface) *MultiplexerManager {
	configmapInformer := cfg.SharedFactory.Core().V1().ConfigMaps().Informer()
	poolScopeMetadata := sets.New[string]()
	for i := range cfg.PoolScopeResources {
		poolScopeMetadata.Insert(cfg.PoolScopeResources[i].String())
	}
	klog.Infof("pool scope resources: %v", poolScopeMetadata)

	m := &MultiplexerManager{
		filterStoreManager:            newFilterStoreManager(cfg, storageProvider),
		healthCheckerForLeaders:       healthCheckerForLeaders,
		loadBalancerForLeaders:        cfg.LoadBalancerForLeaderHub,
		poolScopeMetadata:             poolScopeMetadata,
		lazyLoadedGVRCache:            make(map[string]Interface),
		lazyLoadedGVRCacheDestroyFunc: make(map[string]func()),
		leaderAddresses:               sets.New[string](),
		portForLeaderHub:              cfg.PortForMultiplexer,
		nodeName:                      cfg.NodeName,
		multiplexerUserAgent:          hubutil.MultiplexerProxyClientUserAgentPrefix + cfg.NodeName,
		configMapSynced:               configmapInformer.HasSynced,
	}

	// prepare leader-hub-{pool-name} configmap event handler
	leaderHubConfigMapName := fmt.Sprintf("leader-hub-%s", cfg.NodePoolName)
	configmapInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			cfg, ok := obj.(*corev1.ConfigMap)
			if ok && cfg.Name == leaderHubConfigMapName {
				return true
			}
			return false
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    m.addConfigmap,
			UpdateFunc: m.updateConfigmap,
			// skip DeleteFunc, because only NodePool deletion will cause to delete this configmap.
		},
	})
	return m
}

func (m *MultiplexerManager) addConfigmap(obj interface{}) {
	cm, _ := obj.(*corev1.ConfigMap)

	m.updateLeaderHubConfiguration(cm)
	klog.Infof(
		"after added configmap, source for pool scope metadata: %s, pool scope metadata: %v",
		m.sourceForPoolScopeMetadata,
		m.poolScopeMetadata,
	)
}

func (m *MultiplexerManager) updateConfigmap(oldObj, newObj interface{}) {
	oldCM, _ := oldObj.(*corev1.ConfigMap)
	newCM, _ := newObj.(*corev1.ConfigMap)

	if maps.Equal(oldCM.Data, newCM.Data) {
		return
	}

	m.updateLeaderHubConfiguration(newCM)
	klog.Infof(
		"after updated configmap, source for pool scope metadata: %s, pool scope metadata: %v",
		m.sourceForPoolScopeMetadata,
		m.poolScopeMetadata,
	)
}

func (m *MultiplexerManager) updateLeaderHubConfiguration(cm *corev1.ConfigMap) {
	newPoolScopeMetadata := sets.New[string]()
	if len(cm.Data[PoolScopeMetadataKey]) != 0 {
		for _, part := range strings.Split(cm.Data[PoolScopeMetadataKey], ",") {
			subParts := strings.Split(part, "/")
			if len(subParts) == 3 {
				gvr := schema.GroupVersionResource{
					Group:    subParts[0],
					Version:  subParts[1],
					Resource: subParts[2],
				}
				newPoolScopeMetadata.Insert(gvr.String())
			}
		}
	}

	newLeaderNames := sets.New[string]()
	newLeaderAddresses := sets.New[string]()
	if len(cm.Data[LeaderEndpointsKey]) != 0 {
		for _, part := range strings.Split(cm.Data[LeaderEndpointsKey], ",") {
			subParts := strings.Split(part, "/")
			if len(subParts) == 2 {
				newLeaderNames.Insert(subParts[0])
				newLeaderAddresses.Insert(subParts[1])
			}
		}
	}

	newSource := APIServerSourceForPoolScopeMetadata
	// enable-leader-election is enabled and node is not elected as leader hub,
	// multiplexer will list/watch pool scope metadata from leader yurthub.
	// otherwise, multiplexer will list/watch pool scope metadata from cloud kube-apiserver.
	if cm.Data[EnableLeaderElection] == "true" && len(newLeaderAddresses) != 0 && !newLeaderNames.Has(m.nodeName) {
		newSource = PoolSourceForPoolScopeMetadata
	}

	// LeaderHubEndpoints are changed, related health checker and load balancer are need to be updated.
	if !m.leaderAddresses.Equal(newLeaderAddresses) {
		servers := m.resolveLeaderHubServers(newLeaderAddresses)
		m.healthCheckerForLeaders.UpdateBackends(servers)
		m.loadBalancerForLeaders.UpdateBackends(servers)
		m.leaderAddresses = newLeaderAddresses
	}

	if m.sourceForPoolScopeMetadata == newSource &&
		m.poolScopeMetadata.Equal(newPoolScopeMetadata) {
		return
	}

	// if pool scope metadata are removed, related GVR cache should be destroyed.
	deletedPoolScopeMetadata := m.poolScopeMetadata.Difference(newPoolScopeMetadata)

	m.Lock()
	defer m.Unlock()
	m.sourceForPoolScopeMetadata = newSource
	m.poolScopeMetadata = newPoolScopeMetadata
	for _, gvrStr := range deletedPoolScopeMetadata.UnsortedList() {
		if destroyFunc, ok := m.lazyLoadedGVRCacheDestroyFunc[gvrStr]; ok {
			destroyFunc()
		}
		delete(m.lazyLoadedGVRCacheDestroyFunc, gvrStr)
		delete(m.lazyLoadedGVRCache, gvrStr)
	}
}

func (m *MultiplexerManager) HasSynced() bool {
	return m.configMapSynced()
}

func (m *MultiplexerManager) SourceForPoolScopeMetadata() string {
	m.RLock()
	defer m.RUnlock()
	return m.sourceForPoolScopeMetadata
}

// ResolveRequestForPoolScopeMetadata is used for resolving requests for list/watching pool scope metadata.
// there are two return values:
// isRequestForPoolScopeMetadata: specify whether the request list/watch pool scope metadata or not. if true, it is a request for pool scope metadata, otherwise, it's not.
// forwardRequestForPoolScopeMetadata: specify whether the request for pool scope metadata should be forwarded or can be served by multiplexer.
// if true, it means request should be forwarded, otherwise, request should be served by multiplexer.
// by the way, return value: forwardRequestForPoolScopeMetadata  can be used when return value: isRequestForPoolScopeMetadata is true.
func (m *MultiplexerManager) ResolveRequestForPoolScopeMetadata(req *http.Request) (isRequestForPoolScopeMetadata bool, forwardRequestForPoolScopeMetadata bool) {
	info, ok := apirequest.RequestInfoFrom(req.Context())
	if !ok {
		isRequestForPoolScopeMetadata = false
		forwardRequestForPoolScopeMetadata = false
		return
	}

	// list/watch requests
	if info.Verb != "list" && info.Verb != "watch" {
		isRequestForPoolScopeMetadata = false
		forwardRequestForPoolScopeMetadata = false
		return
	}

	gvr := schema.GroupVersionResource{
		Group:    info.APIGroup,
		Version:  info.APIVersion,
		Resource: info.Resource,
	}

	m.RLock()
	isRequestForPoolScopeMetadata = m.poolScopeMetadata.Has(gvr.String())
	m.RUnlock()

	// the request comes from multiplexer manager, so the request should be forwarded instead of serving by local multiplexer.
	if req.UserAgent() == m.multiplexerUserAgent {
		forwardRequestForPoolScopeMetadata = true
	} else {
		forwardRequestForPoolScopeMetadata = false
	}
	return
}

func (m *MultiplexerManager) resolveLeaderHubServers(leaderAddresses sets.Set[string]) []*url.URL {
	servers := make([]*url.URL, 0, leaderAddresses.Len())
	for _, internalIP := range leaderAddresses.UnsortedList() {
		u, err := url.Parse(fmt.Sprintf("https://%s:%d", internalIP, m.portForLeaderHub))
		if err != nil {
			klog.Errorf("couldn't parse url(%s), %v", fmt.Sprintf("https://%s:%d", internalIP, m.portForLeaderHub), err)
			continue
		}
		servers = append(servers, u)
	}
	return servers
}

func (m *MultiplexerManager) Ready(gvr *schema.GroupVersionResource) bool {
	fs, err := m.filterStoreManager.FilterStore(gvr)
	if err != nil {
		klog.Errorf("failed to get resource cache for gvr %s, %v", gvr.String(), err)
		return false
	}

	return fs.ReadinessCheck() == nil
}

func (m *MultiplexerManager) ResourceStore(gvr *schema.GroupVersionResource) (rest.Storage, error) {
	return m.filterStoreManager.FilterStore(gvr)
}
