/*
Copyright 2023 The OpenYurt Authors.

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

package resources

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type PoolScopeResourcesManger struct {
	validPoolScopedResources     map[string]*verifiablePoolScopeResource
	validPoolScopedResourcesLock sync.RWMutex
	k8sClient                    kubernetes.Interface
	hasSynced                    func() bool
}

var poolScopeResourcesManger *PoolScopeResourcesManger

func InitPoolScopeResourcesManger(client kubernetes.Interface, factory informers.SharedInformerFactory) *PoolScopeResourcesManger {
	poolScopeResourcesManger = &PoolScopeResourcesManger{
		k8sClient:                client,
		validPoolScopedResources: make(map[string]*verifiablePoolScopeResource),
	}
	configmapInformer := factory.Core().V1().ConfigMaps().Informer()
	configmapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: poolScopeResourcesManger.addConfigmap,
		// todo: now we do not support update of pool scope resources definition
	})
	poolScopeResourcesManger.hasSynced = configmapInformer.HasSynced

	klog.Infof("init pool scope resources manager")

	poolScopeResourcesManger.setVerifiableGVRs(poolScopeResourcesManger.getInitPoolScopeResources()...)
	return poolScopeResourcesManger
}

func WaitUntilPoolScopeResourcesSync(ctx context.Context) {
	cache.WaitForCacheSync(ctx.Done(), poolScopeResourcesManger.hasSynced)
}

func IsPoolScopeResources(info *apirequest.RequestInfo) bool {
	if info == nil || poolScopeResourcesManger == nil {
		return false
	}
	_, ok := poolScopeResourcesManger.validPoolScopedResources[schema.GroupVersionResource{
		Group:    info.APIGroup,
		Version:  info.APIVersion,
		Resource: info.Resource,
	}.String()]
	return ok
}

func GetPoolScopeResources() []schema.GroupVersionResource {
	if poolScopeResourcesManger == nil {
		return []schema.GroupVersionResource{}
	}
	return poolScopeResourcesManger.getPoolScopeResources()
}

func (m *PoolScopeResourcesManger) getPoolScopeResources() []schema.GroupVersionResource {
	poolScopeResources := make([]schema.GroupVersionResource, 0)
	m.validPoolScopedResourcesLock.RLock()
	defer m.validPoolScopedResourcesLock.RUnlock()
	for _, v := range m.validPoolScopedResources {
		poolScopeResources = append(poolScopeResources, v.GroupVersionResource)
	}
	return poolScopeResources
}

// addVerifiableGVRs add given gvrs to validPoolScopedResources map
func (m *PoolScopeResourcesManger) addVerifiableGVRs(gvrs ...*verifiablePoolScopeResource) {
	m.validPoolScopedResourcesLock.Lock()
	defer m.validPoolScopedResourcesLock.Unlock()
	for _, gvr := range gvrs {
		if ok, errMsg := gvr.Verify(); ok {
			m.validPoolScopedResources[gvr.String()] = gvr
			klog.Infof("PoolScopeResourcesManger add gvr %s success", gvr.String())
		} else {
			klog.Warningf("PoolScopeResourcesManger add gvr %s failed, because %s", gvr.String(), errMsg)
		}
	}
}

// addVerifiableGVRs clear validPoolScopedResources and set given gvrs to validPoolScopedResources map
func (m *PoolScopeResourcesManger) setVerifiableGVRs(gvrs ...*verifiablePoolScopeResource) {
	m.validPoolScopedResourcesLock.Lock()
	defer m.validPoolScopedResourcesLock.Unlock()
	m.validPoolScopedResources = make(map[string]*verifiablePoolScopeResource)
	for _, gvr := range gvrs {
		if ok, errMsg := gvr.Verify(); ok {
			m.validPoolScopedResources[gvr.String()] = gvr
			klog.Infof("PoolScopeResourcesManger update gvr %s success", gvr.String())
		} else {
			klog.Warningf("PoolScopeResourcesManger update gvr %s failed, because %s", gvr.String(), errMsg)
		}
	}
}

func (m *PoolScopeResourcesManger) addConfigmap(obj interface{}) {
	cfg, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	poolScopeResources := cfg.Data[util.PoolScopeResourcesKey]
	poolScopeResourcesGVRs := make([]schema.GroupVersionResource, 0)
	verifiablePoolScopeResourcesGVRs := make([]*verifiablePoolScopeResource, 0)
	if err := json.Unmarshal([]byte(poolScopeResources), &poolScopeResourcesGVRs); err != nil {
		klog.Errorf("PoolScopeResourcesManger unmarshal poolScopeResources %s failed with error = %s",
			poolScopeResources, err.Error())
		return
	}
	klog.Infof("PoolScopeResourcesManger add configured pool scope resources %v", poolScopeResourcesGVRs)
	for _, v := range poolScopeResourcesGVRs {
		verifiablePoolScopeResourcesGVRs = append(verifiablePoolScopeResourcesGVRs,
			newVerifiablePoolScopeResource(v, m.getGroupVersionVerifyFunction(m.k8sClient)))
	}
	m.addVerifiableGVRs(verifiablePoolScopeResourcesGVRs...)
}

func (m *PoolScopeResourcesManger) getGroupVersionVerifyFunction(client kubernetes.Interface) func(gvr schema.GroupVersionResource) (bool, string) {
	return func(gvr schema.GroupVersionResource) (bool, string) {
		maxRetry := 3
		duration := time.Second * 5
		counter := 0
		var err error
		for counter <= maxRetry {
			if _, err = client.Discovery().ServerResourcesForGroupVersion(gvr.GroupVersion().String()); err == nil {
				return true, "" // gvr found
			}
			if apierrors.IsNotFound(err) {
				return false, err.Error() // gvr not found
			}
			// unexpected error, retry
			counter++
			time.Sleep(duration)
		}
		return false, err.Error()
	}
}

func (m *PoolScopeResourcesManger) getInitPoolScopeResources() []*verifiablePoolScopeResource {
	return []*verifiablePoolScopeResource{
		newVerifiablePoolScopeResource(
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"},
			m.getGroupVersionVerifyFunction(m.k8sClient)),
		newVerifiablePoolScopeResource(
			schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
			m.getGroupVersionVerifyFunction(m.k8sClient)),
		newVerifiablePoolScopeResource(
			schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1beta1", Resource: "endpointslices"},
			m.getGroupVersionVerifyFunction(m.k8sClient)),
	}
}
