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

package cachemanager

import (
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	sepForAgent = ","
)

type CacheAgent struct {
	sync.Mutex
	agents sets.String
	store  StorageWrapper
}

func NewCacheAgents(informerFactory informers.SharedInformerFactory, store StorageWrapper) *CacheAgent {
	ca := &CacheAgent{
		agents: sets.NewString(util.DefaultCacheAgents...),
		store:  store,
	}
	configmapInformer := informerFactory.Core().V1().ConfigMaps().Informer()
	configmapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ca.addConfigmap,
		UpdateFunc: ca.updateConfigmap,
		DeleteFunc: ca.deleteConfigmap,
	})

	klog.Infof("init cache agents to %v", ca.agents)
	return ca
}

func (ca *CacheAgent) HasAny(items ...string) bool {
	return ca.agents.HasAny(items...)
}

func (ca *CacheAgent) addConfigmap(obj interface{}) {
	cfg, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	deletedAgents := ca.updateCacheAgents(cfg.Data[util.CacheUserAgentsKey], "add")
	ca.deleteAgentCache(deletedAgents)
}

func (ca *CacheAgent) updateConfigmap(oldObj, newObj interface{}) {
	oldCfg, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	newCfg, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	if oldCfg.Data[util.CacheUserAgentsKey] == newCfg.Data[util.CacheUserAgentsKey] {
		return
	}

	deletedAgents := ca.updateCacheAgents(newCfg.Data[util.CacheUserAgentsKey], "update")
	ca.deleteAgentCache(deletedAgents)
}

func (ca *CacheAgent) deleteConfigmap(obj interface{}) {
	_, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	deletedAgents := ca.updateCacheAgents("", "delete")
	ca.deleteAgentCache(deletedAgents)
}

// updateCacheAgents update cache agents
func (ca *CacheAgent) updateCacheAgents(cacheAgents, action string) sets.String {
	newAgents := sets.NewString(util.DefaultCacheAgents...)
	for _, agent := range strings.Split(cacheAgents, sepForAgent) {
		agent = strings.TrimSpace(agent)
		if len(agent) != 0 {
			newAgents.Insert(agent)
		}
	}

	ca.Lock()
	defer ca.Unlock()

	if ca.agents.Equal(newAgents) {
		return sets.String{}
	}

	// get deleted and added agents
	deletedAgents := ca.agents.Difference(newAgents)
	ca.agents = newAgents

	klog.Infof("current cache agents: %v after %s, deleted agents: %v", ca.agents, action, deletedAgents)

	// return deleted agents
	return deletedAgents
}

func (ca *CacheAgent) deleteAgentCache(deletedAgents sets.String) {
	// delete cache data for deleted agents
	if deletedAgents.Len() > 0 {
		components := deletedAgents.List()
		for i := range components {
			if err := ca.store.DeleteComponentResources(components[i]); err != nil {
				klog.Errorf("could not cleanup cache for deleted agent(%s), %v", components[i], err)
			} else {
				klog.Infof("cleanup cache for agent(%s) successfully", components[i])
			}
		}
	}
}
