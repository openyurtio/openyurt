/*
Copyright 2025 The OpenYurt Authors.

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

package configuration

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	cacheUserAgentsKey = "cache_agents"
	sepForAgent        = ","
)

var (
	defaultCacheAgents = []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName()}
)

// Manager is used for managing all configurations of Yurthub in yurt-hub-cfg configmap.
// This configuration configmap includes configurations of cache agents and filters. I'm sure that new
// configurations will be added according to user's new requirements.
type Manager struct {
	sync.RWMutex
	baseAgents       []string
	allCacheAgents   sets.Set[string]
	baseKeyToFilters map[string][]string
	reqKeyToFilters  map[string][]string
	configMapSynced  cache.InformerSynced
}

func NewConfigurationManager(nodeName string, sharedFactory informers.SharedInformerFactory) *Manager {
	configmapInformer := sharedFactory.Core().V1().ConfigMaps().Informer()
	m := &Manager{
		baseAgents:       append(defaultCacheAgents, util.MultiplexerProxyClientUserAgentPrefix+nodeName),
		allCacheAgents:   sets.New[string](),
		baseKeyToFilters: make(map[string][]string),
		reqKeyToFilters:  make(map[string][]string),
		configMapSynced:  configmapInformer.HasSynced,
	}

	// init cache agents
	m.updateCacheAgents("", "init")
	for filterName, req := range options.FilterToComponentsResourcesAndVerbs {
		for _, comp := range req.DefaultComponents {
			for resource, verbs := range req.ResourceAndVerbs {
				for _, verb := range verbs {
					if key := reqKey(comp, verb, resource); len(key) != 0 {
						if _, ok := m.baseKeyToFilters[key]; !ok {
							m.baseKeyToFilters[key] = []string{filterName}
						} else {
							m.baseKeyToFilters[key] = append(m.baseKeyToFilters[key], filterName)
						}
					}
				}
			}
		}
	}
	// init filter settings
	m.updateFilterSettings(map[string]string{}, "init")

	// prepare configmap event handler
	configmapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.addConfigmap,
		UpdateFunc: m.updateConfigmap,
		DeleteFunc: m.deleteConfigmap,
	})
	return m
}

// HasSynced is used for checking that configuration of Yurthub has been loaded completed or not.
func (m *Manager) HasSynced() bool {
	return m.configMapSynced()
}

// ListAllCacheAgents is used for listing all cache agents.
func (m *Manager) ListAllCacheAgents() []string {
	m.RLock()
	defer m.RUnlock()
	return m.allCacheAgents.UnsortedList()
}

// IsCacheable is used for checking that http response of specified component
// should be cached on the local disk or not.
func (m *Manager) IsCacheable(comp string) bool {
	if strings.Contains(comp, "/") {
		index := strings.Index(comp, "/")
		if index != -1 {
			comp = comp[:index]
		}
	}

	m.RLock()
	defer m.RUnlock()
	return m.allCacheAgents.HasAny("*", comp)
}

// FindFiltersFor is used for finding all filters for the specified request.
// the return value represents all the filter names for the request.
func (m *Manager) FindFiltersFor(req *http.Request) []string {
	key := getKeyByRequest(req)
	if len(key) == 0 {
		return []string{}
	}

	m.RLock()
	defer m.RUnlock()
	if filters, ok := m.reqKeyToFilters[key]; ok {
		return filters
	}
	return []string{}
}

func (m *Manager) addConfigmap(obj interface{}) {
	cfg, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	m.updateCacheAgents(cfg.Data[cacheUserAgentsKey], "add")
	m.updateFilterSettings(cfg.Data, "add")
}

func (m *Manager) updateConfigmap(oldObj, newObj interface{}) {
	oldCfg, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	newCfg, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	if oldCfg.Data[cacheUserAgentsKey] != newCfg.Data[cacheUserAgentsKey] {
		m.updateCacheAgents(newCfg.Data[cacheUserAgentsKey], "update")
	}

	if filterSettingsChanged(oldCfg.Data, newCfg.Data) {
		m.updateFilterSettings(newCfg.Data, "update")
	}
}

func (m *Manager) deleteConfigmap(obj interface{}) {
	_, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	m.updateCacheAgents("", "delete")
	m.updateFilterSettings(map[string]string{}, "delete")
}

// updateCacheAgents update cache agents
// todo: cache on the local disk should be removed when agent is deleted.
func (m *Manager) updateCacheAgents(cacheAgents, action string) {
	newAgents := make([]string, 0)
	newAgents = append(newAgents, m.baseAgents...)
	for _, agent := range strings.Split(cacheAgents, sepForAgent) {
		agent = strings.TrimSpace(agent)
		if len(agent) != 0 {
			newAgents = append(newAgents, agent)
		}
	}

	klog.Infof("After action %s, the cache agents are as follows: %v", action, newAgents)
	m.Lock()
	defer m.Unlock()
	m.allCacheAgents.Clear()
	m.allCacheAgents.Insert(newAgents...)
}

// filterSettingsChanged is used to verify filter setting is changed or not.
func filterSettingsChanged(old, new map[string]string) bool {
	oldCopy := make(map[string]string)
	newCopy := make(map[string]string)
	for key, val := range old {
		if _, ok := options.FilterToComponentsResourcesAndVerbs[key]; ok {
			oldCopy[key] = val
		}
	}

	for key, val := range new {
		if _, ok := options.FilterToComponentsResourcesAndVerbs[key]; ok {
			newCopy[key] = val
		}
	}

	// if filter setting of old and new equal, return false.
	// vice versa, return true.
	return !reflect.DeepEqual(oldCopy, newCopy)
}

func (m *Manager) updateFilterSettings(cmData map[string]string, action string) {
	// prepare the default filter settings
	reqKeyToFilterSet := make(map[string]sets.Set[string])
	for key, filterNames := range m.baseKeyToFilters {
		reqKeyToFilterSet[key] = sets.New[string](filterNames...)
	}

	// add filter settings from configmap
	for filterName, components := range cmData {
		if req, ok := options.FilterToComponentsResourcesAndVerbs[filterName]; ok {
			for _, comp := range strings.Split(components, sepForAgent) {
				for resource, verbs := range req.ResourceAndVerbs {
					for _, verb := range verbs {
						if key := reqKey(comp, verb, resource); len(key) != 0 {
							if _, ok := reqKeyToFilterSet[key]; !ok {
								reqKeyToFilterSet[key] = sets.New[string](filterName)
							} else {
								reqKeyToFilterSet[key].Insert(filterName)
							}
						}
					}
				}
			}
		}
	}

	reqKeyToFilters := make(map[string][]string)
	for key, filterSet := range reqKeyToFilterSet {
		reqKeyToFilters[key] = filterSet.UnsortedList()
	}

	klog.Infof("After action %s, the filter settings are as follows: %v", action, reqKeyToFilters)
	m.Lock()
	defer m.Unlock()
	m.reqKeyToFilters = reqKeyToFilters
}

// getKeyByRequest returns reqKey for specified request.
func getKeyByRequest(req *http.Request) string {
	var key string
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok {
		return key
	}

	if strings.Contains(comp, "/") {
		index := strings.Index(comp, "/")
		if index != -1 {
			comp = comp[:index]
		}
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok {
		return key
	}

	return reqKey(comp, info.Verb, info.Resource)
}

// reqKey is made up by comp and verb, resource
func reqKey(comp, verb, resource string) string {
	if len(comp) == 0 || len(resource) == 0 || len(verb) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimSpace(comp), strings.TrimSpace(verb), strings.TrimSpace(resource))
}
