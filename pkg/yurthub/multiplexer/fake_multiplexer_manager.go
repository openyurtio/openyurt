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
