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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
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

var (
	KeyFunc = func(obj runtime.Object) (string, error) {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return "", err
		}

		name := accessor.GetName()
		if len(name) == 0 {
			return "", apierrors.NewBadRequest("Name parameter required.")
		}

		ns := accessor.GetNamespace()
		if len(ns) == 0 {
			return "/" + name, nil
		}
		return "/" + ns + "/" + name, nil
	}

	AttrsFunc = func(obj runtime.Object) (labels.Set, fields.Set, error) {
		metadata, err := meta.Accessor(obj)
		if err != nil {
			return nil, nil, err
		}

		var fieldSet fields.Set
		if len(metadata.GetNamespace()) > 0 {
			fieldSet = fields.Set{
				"metadata.name":      metadata.GetName(),
				"metadata.namespace": metadata.GetNamespace(),
			}
		} else {
			fieldSet = fields.Set{
				"metadata.name": metadata.GetName(),
			}
		}

		return labels.Set(metadata.GetLabels()), fieldSet, nil
	}
)

type MultiplexerManager struct {
	restStoreProvider       ystorage.StorageProvider
	restMapper              *hubmeta.RESTMapperManager
	healthCheckerForLeaders healthchecker.Interface
	loadBalancerForLeaders  remote.LoadBalancer
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
	restStoreProvider ystorage.StorageProvider,
	healthCheckerForLeaders healthchecker.Interface) *MultiplexerManager {
	configmapInformer := cfg.SharedFactory.Core().V1().ConfigMaps().Informer()
	poolScopeMetadata := sets.New[string]()
	for i := range cfg.PoolScopeResources {
		poolScopeMetadata.Insert(cfg.PoolScopeResources[i].String())
	}
	klog.Infof("pool scope resources: %v", poolScopeMetadata)

	m := &MultiplexerManager{
		restStoreProvider:             restStoreProvider,
		restMapper:                    cfg.RESTMapperManager,
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
	klog.Infof("after added configmap, source for pool scope metadata: %s, pool scope metadata: %v", m.sourceForPoolScopeMetadata, m.poolScopeMetadata)
}

func (m *MultiplexerManager) updateConfigmap(oldObj, newObj interface{}) {
	oldCM, _ := oldObj.(*corev1.ConfigMap)
	newCM, _ := newObj.(*corev1.ConfigMap)

	if maps.Equal(oldCM.Data, newCM.Data) {
		return
	}

	m.updateLeaderHubConfiguration(newCM)
	klog.Infof("after updated configmap, source for pool scope metadata: %s, pool scope metadata: %v", m.sourceForPoolScopeMetadata, m.poolScopeMetadata)
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

func (m *MultiplexerManager) IsRequestForPoolScopeMetadata(req *http.Request) bool {
	// the requests from multiplexer manager, recongnize it as not multiplexer request.
	if req.UserAgent() == m.multiplexerUserAgent {
		return false
	}

	info, ok := apirequest.RequestInfoFrom(req.Context())
	if !ok {
		return false
	}

	// list/watch requests
	if info.Verb != "list" && info.Verb != "watch" {
		return false
	}

	gvr := schema.GroupVersionResource{
		Group:    info.APIGroup,
		Version:  info.APIVersion,
		Resource: info.Resource,
	}

	m.RLock()
	defer m.RUnlock()
	return m.poolScopeMetadata.Has(gvr.String())
}

func (m *MultiplexerManager) Ready(gvr *schema.GroupVersionResource) bool {
	rc, _, err := m.ResourceCache(gvr)
	if err != nil {
		klog.Errorf("failed to get resource cache for gvr %s, %v", gvr.String(), err)
		return false
	}

	return rc.ReadinessCheck() == nil
}

// ResourceCache is used for preparing cache for specified gvr.
// The cache is loaded in a lazy mode, this means cache will not be loaded when yurthub initializes,
// and cache will only be loaded when corresponding request is received.
func (m *MultiplexerManager) ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error) {
	// use read lock to get resource cache, so requests can not be blocked.
	m.RLock()
	rc, exists := m.lazyLoadedGVRCache[gvr.String()]
	destroyFunc := m.lazyLoadedGVRCacheDestroyFunc[gvr.String()]
	m.RUnlock()

	if exists {
		return rc, destroyFunc, nil
	}

	// resource cache doesn't exist, initialize multiplexer cache for gvr
	m.Lock()
	defer m.Unlock()

	// maybe multiple requests are served to initialize multiplexer cache at the same time,
	// so we need to check the cache another time before initializing cache.
	if rc, exists := m.lazyLoadedGVRCache[gvr.String()]; exists {
		return rc, m.lazyLoadedGVRCacheDestroyFunc[gvr.String()], nil
	}

	klog.Infof("start initializing multiplexer cache for gvr: %s", gvr.String())
	restStore, err := m.restStoreProvider.ResourceStorage(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get rest store")
	}

	resourceCacheConfig, err := m.resourceCacheConfig(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate resource cache config")
	}

	rc, destroy, err := NewResourceCache(restStore, gvr, resourceCacheConfig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to new resource cache")
	}

	m.lazyLoadedGVRCache[gvr.String()] = rc
	m.lazyLoadedGVRCacheDestroyFunc[gvr.String()] = destroy

	return rc, destroy, nil
}

func (m *MultiplexerManager) resourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error) {
	gvk, listGVK, err := m.convertToGVK(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert to gvk from gvr %s", gvr.String())
	}

	return m.newResourceCacheConfig(gvk, listGVK), nil
}

func (m *MultiplexerManager) convertToGVK(gvr *schema.GroupVersionResource) (schema.GroupVersionKind, schema.GroupVersionKind, error) {
	_, gvk := m.restMapper.KindFor(*gvr)
	if gvk.Empty() {
		return schema.GroupVersionKind{}, schema.GroupVersionKind{}, fmt.Errorf("failed to convert gvk from gvr %s", gvr.String())
	}

	listGvk := schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    gvk.Kind + "List",
	}

	return gvk, listGvk, nil
}

func (m *MultiplexerManager) newResourceCacheConfig(gvk schema.GroupVersionKind,
	listGVK schema.GroupVersionKind) *ResourceCacheConfig {
	return &ResourceCacheConfig{
		NewFunc: func() runtime.Object {
			obj, _ := scheme.Scheme.New(gvk)
			return obj
		},
		NewListFunc: func() (object runtime.Object) {
			objList, _ := scheme.Scheme.New(listGVK)
			return objList
		},
		KeyFunc:      KeyFunc,
		GetAttrsFunc: AttrsFunc,
	}
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
