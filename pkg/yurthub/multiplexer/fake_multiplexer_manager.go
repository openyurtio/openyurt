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

import "k8s.io/apimachinery/pkg/runtime/schema"

type FakeCacheManager struct {
	cacheMap          map[string]Interface
	resourceConfigMap map[string]*ResourceCacheConfig
}

func NewFakeCacheManager(cacheMap map[string]Interface, resourceConfigMap map[string]*ResourceCacheConfig) *FakeCacheManager {
	return &FakeCacheManager{
		cacheMap:          cacheMap,
		resourceConfigMap: resourceConfigMap,
	}
}

func (fcm *FakeCacheManager) ResourceCacheConfig(gvr *schema.GroupVersionResource) (*ResourceCacheConfig, error) {
	return fcm.resourceConfigMap[gvr.String()], nil
}

func (fcm *FakeCacheManager) ResourceCache(gvr *schema.GroupVersionResource) (Interface, func(), error) {
	return fcm.cacheMap[gvr.String()], nil, nil
}
