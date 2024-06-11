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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	ystorage "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

var NamespaceKeyFunc = func(obj runtime.Object) (string, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}

	return registry.NamespaceKeyFunc(request.WithNamespace(request.NewContext(), accessor.GetNamespace()), "", accessor.GetName())
}

var NoNamespaceKeyFunc = func(obj runtime.Object) (string, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}

	return registry.NoNamespaceKeyFunc(request.WithNamespace(request.NewContext(), accessor.GetNamespace()), "", accessor.GetName())
}

type MultiplexerManager interface {
	ResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error)
	ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error)
}

type multiplexerManager struct {
	restStoreManager    ystorage.StorageManager
	discoveryClient     discovery.DiscoveryInterface
	cacheMap            map[string]Interface
	cacheConfigMap      map[string]*ResourceCacheConfig
	cacheDestroyFuncMap map[string]func()
}

func NewShareCacheManager(
	restStoreManager ystorage.StorageManager,
	discoveryClient discovery.DiscoveryInterface) MultiplexerManager {
	return &multiplexerManager{
		restStoreManager:    restStoreManager,
		discoveryClient:     discoveryClient,
		cacheMap:            make(map[string]Interface),
		cacheConfigMap:      make(map[string]*ResourceCacheConfig),
		cacheDestroyFuncMap: make(map[string]func()),
	}
}

func (scm *multiplexerManager) ResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error) {
	if config, ok := scm.cacheConfigMap[gvr.String()]; ok {
		return config, nil
	}

	apiResource, err := scm.getAPIResource(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get api resource from %s", gvr.String())
	}

	gvk, listGVK := scm.convertToGVK(gvr, apiResource)

	config := scm.newResourceCacheConfig(gvk, listGVK, apiResource)

	scm.cacheConfigMap[gvr.String()] = config
	return config, nil
}

func (scm *multiplexerManager) getAPIResource(gvr *schema.GroupVersionResource) (metav1.APIResource, error) {
	resourceList, err := scm.discoveryClient.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		return metav1.APIResource{}, errors.Wrapf(err, "failed to get api resource list for %s", gvr.GroupVersion().String())
	}

	for _, resource := range resourceList.APIResources {
		if resource.Name == gvr.Resource {
			return resource, nil
		}
	}

	return metav1.APIResource{}, errors.Errorf("Not found api resource for %s", gvr.String())
}

func (scm *multiplexerManager) convertToGVK(gvr *schema.GroupVersionResource, resource metav1.APIResource) (schema.GroupVersionKind, schema.GroupVersionKind) {
	gvk := schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    resource.Kind,
	}

	listGvk := schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    resource.Kind + "List",
	}

	return gvk, listGvk
}

func (rcm *multiplexerManager) newResourceCacheConfig(gvk schema.GroupVersionKind,
	listGVK schema.GroupVersionKind,
	apiResource metav1.APIResource) *ResourceCacheConfig {

	resourceCacheConfig := &ResourceCacheConfig{
		NewFunc: func() runtime.Object {
			obj, _ := scheme.Scheme.New(gvk)
			return obj
		},
		NewListFunc: func() (object runtime.Object) {
			objList, _ := scheme.Scheme.New(listGVK)
			return objList
		},
	}

	if apiResource.Namespaced {
		resourceCacheConfig.KeyFunc = NamespaceKeyFunc
		resourceCacheConfig.GetAttrsFunc = kstorage.DefaultNamespaceScopedAttr
		resourceCacheConfig.NamespaceScoped = true
	} else {
		resourceCacheConfig.KeyFunc = NoNamespaceKeyFunc
		resourceCacheConfig.GetAttrsFunc = kstorage.DefaultClusterScopedAttr
		resourceCacheConfig.NamespaceScoped = false
	}

	return resourceCacheConfig
}

func (scm *multiplexerManager) ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error) {
	if sc, ok := scm.cacheMap[gvr.String()]; ok {
		return sc, scm.cacheDestroyFuncMap[gvr.String()], nil
	}

	restStore, err := scm.restStoreManager.ResourceStorage(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get rest store")
	}

	resourceCacheConfig, err := scm.ResourceCacheConfig(gvr)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to generate resource cache config")
	}

	sc, destroy, err := NewResourceCache(restStore, gvr, resourceCacheConfig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to new resource cache")
	}

	scm.cacheMap[gvr.String()] = sc
	scm.cacheDestroyFuncMap[gvr.String()] = destroy

	return sc, destroy, nil
}
