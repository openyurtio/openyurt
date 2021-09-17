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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/sets"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	sepForAgent = ","
)

func (cm *cacheManager) initCacheAgents() error {
	if cm.sharedFactory == nil {
		return nil
	}
	configmapInformer := cm.sharedFactory.InformerFor(&corev1.ConfigMap{}, newConfigmapInformer)
	configmapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cm.addConfigmap,
		UpdateFunc: cm.updateConfigmap,
	})

	klog.Infof("init cache agents to %v", cm.cacheAgents)
	return nil
}

// UpdateCacheAgents update cache agents
func (cm *cacheManager) UpdateCacheAgents(cacheAgents, action string) sets.String {
	newAgents := sets.NewString()
	for _, agent := range strings.Split(cacheAgents, sepForAgent) {
		agent = strings.TrimSpace(agent)
		if len(agent) != 0 {
			newAgents.Insert(agent)
		}
	}

	cm.Lock()
	defer cm.Unlock()
	cm.cacheAgents = cm.cacheAgents.Delete(util.DefaultCacheAgents...)

	if cm.cacheAgents.Equal(newAgents) {
		// add default cache agents
		cm.cacheAgents = cm.cacheAgents.Insert(util.DefaultCacheAgents...)
		return sets.String{}
	}

	// get deleted and added agents
	deletedAgents := cm.cacheAgents.Difference(newAgents)
	addedAgents := newAgents.Difference(cm.cacheAgents)

	// construct new cache agents
	cm.cacheAgents = cm.cacheAgents.Delete(deletedAgents.List()...)
	cm.cacheAgents = cm.cacheAgents.Insert(addedAgents.List()...)
	cm.cacheAgents = cm.cacheAgents.Insert(util.DefaultCacheAgents...)
	klog.Infof("current cache agents: %v after %s, deleted agents: %v", cm.cacheAgents, action, deletedAgents)

	// return deleted agents
	return deletedAgents
}

// ListCacheAgents get all of cache agents
func (cm *cacheManager) ListCacheAgents() []string {
	cm.RLock()
	defer cm.RUnlock()
	agents := make([]string, 0)
	for k := range cm.cacheAgents {
		agents = append(agents, k)
	}
	return agents
}

func newConfigmapInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	selector := fields.Set{"metadata.name": util.YurthubConfigMapName}.String()
	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = selector
	}

	return coreinformers.NewFilteredConfigMapInformer(cs, util.YurtHubNamespace, resyncPeriod, nil, tweakListOptions)
}

func (cm *cacheManager) addConfigmap(obj interface{}) {
	cfg, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	deletedAgents := cm.UpdateCacheAgents(cfg.Data[util.CacheUserAgentsKey], "add")
	cm.deleteAgentCache(deletedAgents)
}

func (cm *cacheManager) updateConfigmap(oldObj, newObj interface{}) {
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

	deletedAgents := cm.UpdateCacheAgents(newCfg.Data[util.CacheUserAgentsKey], "update")
	cm.deleteAgentCache(deletedAgents)
}

func (cm *cacheManager) deleteAgentCache(deletedAgents sets.String) {
	// delete cache data for deleted agents
	if deletedAgents.Len() > 0 {
		keys := deletedAgents.List()
		for i := range keys {
			if err := cm.storage.DeleteCollection(keys[i]); err != nil {
				klog.Errorf("failed to cleanup cache for deleted agent(%s), %v", keys[i], err)
			} else {
				klog.Infof("cleanup cache for agent(%s) successfully", keys[i])
			}
		}
	}
}
