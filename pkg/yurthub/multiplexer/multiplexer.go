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
	"sync"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"

	kmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
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

type MultiplexerManager interface {
	ResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error)
	ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error)
}

type multiplexerManager struct {
	restStoreManager ystorage.StorageProvider
	restMapper       meta.RESTMapper

	cacheLock             sync.RWMutex
	gvrToCache            map[string]Interface
	gvrToCacheDestroyFunc map[string]func()

	cacheConfigLock  sync.RWMutex
	gvrToCacheConfig map[string]*ResourceCacheConfig
}

func NewRequestsMultiplexerManager(
	restStoreManager ystorage.StorageProvider) MultiplexerManager {

	return &multiplexerManager{
		restStoreManager:      restStoreManager,
		restMapper:            kmeta.NewDefaultRESTMapperFromScheme(),
		gvrToCache:            make(map[string]Interface),
		gvrToCacheConfig:      make(map[string]*ResourceCacheConfig),
		gvrToCacheDestroyFunc: make(map[string]func()),
		cacheLock:             sync.RWMutex{},
		cacheConfigLock:       sync.RWMutex{},
	}
}
func (m *multiplexerManager) ResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error) {
	if config, ok := m.tryGetResourceCacheConfig(gvr); ok {
		return config, nil
	}

	gvk, listGVK, err := m.convertToGVK(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert to gvk from gvr %s", gvr.String())
	}

	config := m.newResourceCacheConfig(gvk, listGVK)

	m.saveResourceCacheConfig(gvr, config)
	return config, nil
}

func (m *multiplexerManager) tryGetResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, bool) {
	m.cacheConfigLock.RLock()
	defer m.cacheConfigLock.RUnlock()

	if config, ok := m.gvrToCacheConfig[gvr.String()]; ok {
		return config, true
	}

	return nil, false
}

func (m *multiplexerManager) convertToGVK(gvr *schema.GroupVersionResource) (schema.GroupVersionKind, schema.GroupVersionKind, error) {
	gvk, err := m.restMapper.KindFor(*gvr)
	if err != nil {
		return schema.GroupVersionKind{}, schema.GroupVersionKind{}, errors.Wrapf(err, "failed to convert gvk from gvr %s", gvr.String())
	}

	listGvk := schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    gvk.Kind + "List",
	}

	return gvk, listGvk, nil
}

func (m *multiplexerManager) newResourceCacheConfig(gvk schema.GroupVersionKind,
	listGVK schema.GroupVersionKind) *ResourceCacheConfig {
	resourceCacheConfig := &ResourceCacheConfig{
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

	return resourceCacheConfig
}

func (m *multiplexerManager) saveResourceCacheConfig(gvr *schema.GroupVersionResource, config *ResourceCacheConfig) {
	m.cacheConfigLock.Lock()
	defer m.cacheConfigLock.Unlock()

	m.gvrToCacheConfig[gvr.String()] = config
}

func (m *multiplexerManager) ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error) {
	if sc, destroy, ok := m.tryGetResourceCache(gvr); ok {
		return sc, destroy, nil
	}

	restStore, err := m.restStoreManager.ResourceStorage(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get rest store")
	}

	resourceCacheConfig, err := m.ResourceCacheConfig(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate resource cache config")
	}

	sc, destroy, err := NewResourceCache(restStore, gvr, resourceCacheConfig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to new resource cache")
	}

	m.saveResourceCache(gvr, sc, destroy)

	return sc, destroy, nil
}

func (m *multiplexerManager) tryGetResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), bool) {
	m.cacheLock.RLock()
	defer m.cacheLock.RUnlock()

	if sc, ok := m.gvrToCache[gvr.String()]; ok {
		return sc, m.gvrToCacheDestroyFunc[gvr.String()], true
	}
	return nil, nil, false
}

func (m *multiplexerManager) saveResourceCache(gvr *schema.GroupVersionResource, sc Interface, destroy func()) {
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	m.gvrToCache[gvr.String()] = sc
	m.gvrToCacheDestroyFunc[gvr.String()] = destroy
}
