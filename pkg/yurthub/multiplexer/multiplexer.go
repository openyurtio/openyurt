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
	"sync"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	ystorage "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

var KeyFunc = func(obj runtime.Object) (string, error) {
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

var AttrsFunc = func(obj runtime.Object) (labels.Set, fields.Set, error) {
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

type MultiplexerManager struct {
	restStoreProvider  ystorage.StorageProvider
	restMapper         *hubmeta.RESTMapperManager
	poolScopeMetadatas sets.Set[string]

	cacheLock                     sync.RWMutex
	lazyLoadedGVRCache            map[string]Interface
	lazyLoadedGVRCacheDestroyFunc map[string]func()
}

func NewRequestMultiplexerManager(
	restStoreProvider ystorage.StorageProvider,
	restMapperMgr *hubmeta.RESTMapperManager,
	poolScopeResources []schema.GroupVersionResource) *MultiplexerManager {

	poolScopeMetadatas := sets.New[string]()
	for i := range poolScopeResources {
		poolScopeMetadatas.Insert(poolScopeResources[i].String())
	}
	klog.Infof("pool scope resources: %v", poolScopeMetadatas)

	return &MultiplexerManager{
		restStoreProvider:             restStoreProvider,
		restMapper:                    restMapperMgr,
		poolScopeMetadatas:            poolScopeMetadatas,
		lazyLoadedGVRCache:            make(map[string]Interface),
		lazyLoadedGVRCacheDestroyFunc: make(map[string]func()),
		cacheLock:                     sync.RWMutex{},
	}
}

func (m *MultiplexerManager) IsPoolScopeMetadata(gvr *schema.GroupVersionResource) bool {
	return m.poolScopeMetadatas.Has(gvr.String())
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
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	if rc, ok := m.lazyLoadedGVRCache[gvr.String()]; ok {
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
