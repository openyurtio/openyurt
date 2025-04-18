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

package multiplexer

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	storage2 "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

type filterStoreManager struct {
	sync.RWMutex
	filterStores    map[string]*filterStore
	restMapper      *hubmeta.RESTMapperManager
	storageProvider storage2.StorageProvider
}

func newFilterStoreManager(hubCfg *config.YurtHubConfiguration, sp storage2.StorageProvider) *filterStoreManager {
	return &filterStoreManager{
		filterStores:    make(map[string]*filterStore),
		restMapper:      hubCfg.RESTMapperManager,
		storageProvider: sp,
	}
}

func (fsm *filterStoreManager) FilterStore(gvr *schema.GroupVersionResource) (*filterStore, error) {
	fsm.RLock()
	fs, exist := fsm.filterStores[gvr.String()]
	fsm.RUnlock()
	if exist {
		return fs, nil
	}

	fsm.Lock()
	defer fsm.Unlock()
	fs, exist = fsm.filterStores[gvr.String()]
	if exist {
		return fs, nil
	}

	store, err := fsm.genericStore(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to new generic store")
	}
	fs = &filterStore{
		gvr:   gvr,
		store: store,
	}

	fsm.filterStores[gvr.String()] = fs

	return fs, nil
}

func (fsm *filterStoreManager) genericStore(gvr *schema.GroupVersionResource) (*registry.Store, error) {
	gvk, listGVK, err := fsm.convertToGVK(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert gvr(%s) to gvk", gvr)
	}

	newFunc, newListFunc := fsm.getNewFunc(gvk, listGVK)

	cache, destroy, err := fsm.resourceCache(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get gvr(%s) cache", gvr)
	}

	return &registry.Store{
		NewFunc:     newFunc,
		NewListFunc: newListFunc,
		KeyFunc:     resourceKeyFunc,
		DestroyFunc: destroy,
		KeyRootFunc: resourceKeyRootFunc,
		Storage: registry.DryRunnableStorage{
			Storage: cache,
		},
		PredicateFunc:      getMatchFunc(gvr),
		ReadinessCheckFunc: cache.ReadinessCheck,
	}, nil
}

func (fsm *filterStoreManager) resourceCache(gvr *schema.GroupVersionResource) (storage.Interface, func(), error) {
	klog.Infof("start initializing multiplexer cache for gvr: %s", gvr.String())
	restStore, err := fsm.storageProvider.ResourceStorage(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get rest store")
	}

	resourceCacheConfig, err := fsm.newResourceCacheConfig(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate resource cache config")
	}

	return newResourceCache(restStore, gvr, resourceCacheConfig)
}

func (fsm *filterStoreManager) newResourceCacheConfig(gvr *schema.GroupVersionResource) (*resourceCacheConfig, error) {
	gvk, listGVK, err := fsm.convertToGVK(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert to gvk from gvr %s", gvr.String())
	}
	newFunc, newListFunc := fsm.getNewFunc(gvk, listGVK)

	return &resourceCacheConfig{
		KeyFunc:      keyFunc,
		NewFunc:      newFunc,
		NewListFunc:  newListFunc,
		GetAttrsFunc: GetAttrsFunc(gvr),
	}, nil
}

func (fsm *filterStoreManager) getNewFunc(gvk, listGvk schema.GroupVersionKind) (func() runtime.Object, func() runtime.Object) {
	return func() runtime.Object {
			obj, _ := scheme.Scheme.New(gvk)
			return obj
		},
		func() (object runtime.Object) {
			objList, _ := scheme.Scheme.New(listGvk)
			return objList
		}
}

func (fsm *filterStoreManager) convertToGVK(gvr *schema.GroupVersionResource) (schema.GroupVersionKind, schema.GroupVersionKind, error) {
	_, gvk := fsm.restMapper.KindFor(*gvr)
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

func (fsm *filterStoreManager) DeleteFilterStore(gvrStr string) {
	fsm.Lock()
	defer fsm.Unlock()

	if _, exist := fsm.filterStores[gvrStr]; exist {
		delete(fsm.filterStores, gvrStr)
	}
}
