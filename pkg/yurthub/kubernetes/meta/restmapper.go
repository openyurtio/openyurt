/*
Copyright 2021 The OpenYurt Authors.

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

package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

const (
	CacheDynamicRESTMapperKey = "_internal/restmapper/cache-crd-restmapper.conf"
	SepForGVR                 = "/"
)

var (
	// unsafeSchemeRESTMapper is used to store the mapping relationship between GVK and GVR in scheme
	// It is not updatable and is only used for the judgment of scheme resources
	unsafeSchemeRESTMapper = NewDefaultRESTMapperFromScheme()
	ErrGVRNotRecognized    = errors.New("GroupVersionResource is not recognized")
)

// RESTMapperManager is responsible for managing different kind of RESTMapper
type RESTMapperManager struct {
	sync.RWMutex
	storage storage.Store
	// UnsafeDefaultRESTMapper is used to save the GVK and GVR mapping relationships of built-in resources
	unsafeDefaultRESTMapper *meta.DefaultRESTMapper
	// dynamicRESTMapper is used to save the GVK and GVR mapping relationships of Custom Resources
	dynamicRESTMapper map[schema.GroupVersionResource]schema.GroupVersionKind
}

func NewDefaultRESTMapperFromScheme() *meta.DefaultRESTMapper {
	s := scheme.Scheme
	defaultGroupVersions := s.PrioritizedVersionsAllGroups()
	mapper := meta.NewDefaultRESTMapper(defaultGroupVersions)
	// enumerate all supported versions, get the kinds, and register with the mapper how to address
	// our resources.
	for _, gv := range defaultGroupVersions {
		for kind := range s.KnownTypes(gv) {
			// Only need to process non-list resources
			if !strings.HasSuffix(kind, "List") {
				// Since RESTMapper is only used for mapping GVR to GVK information,
				// the scope field is not involved in actual use,
				// so all scope are currently set to meta.RESTScopeNamespace
				scope := meta.RESTScopeNamespace
				mapper.Add(gv.WithKind(kind), scope)
			}
		}
	}
	return mapper
}

func NewRESTMapperManager(storage storage.Store) *RESTMapperManager {
	var dm map[schema.GroupVersionResource]schema.GroupVersionKind
	// Recover the mapping relationship between GVR and GVK from the hard disk
	b, err := storage.Get(CacheDynamicRESTMapperKey)
	if err == nil && len(b) != 0 {
		dm = unmarshalDynamicRESTMapper(b)
		klog.Infof("reset DynamicRESTMapper to %v", dm)
	} else {
		dm = make(map[schema.GroupVersionResource]schema.GroupVersionKind)
		klog.Infof("initialize an empty DynamicRESTMapper")
	}

	return &RESTMapperManager{
		unsafeDefaultRESTMapper: unsafeSchemeRESTMapper,
		dynamicRESTMapper:       dm,
		storage:                 storage,
	}
}

// Obtain gvk according to gvr in dynamicRESTMapper
func (rm *RESTMapperManager) dynamicKindFor(gvr schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	hasResource := len(gvr.Resource) > 0
	hasGroup := len(gvr.Group) > 0
	hasVersion := len(gvr.Version) > 0

	if !hasResource || !hasGroup {
		return schema.GroupVersionKind{}, fmt.Errorf("a resource and group must be present, got: %v", gvr)
	}

	rm.RLock()
	defer rm.RUnlock()
	if hasVersion {
		// fully qualified. Find the exact match
		kind, exists := rm.dynamicRESTMapper[gvr]
		if exists {
			return kind, nil
		}
	} else {
		requestedGroupResource := gvr.GroupResource()
		for currResource, currKind := range rm.dynamicRESTMapper {
			if currResource.GroupResource() == requestedGroupResource {
				return currKind, nil
			}
		}
	}
	return schema.GroupVersionKind{}, fmt.Errorf("no matches for %v", gvr)
}

// Used to delete the mapping relationship between GVR and GVK in dynamicRESTMapper
func (rm *RESTMapperManager) deleteKind(gvk schema.GroupVersionKind) error {
	kindName := strings.TrimSuffix(gvk.Kind, "List")
	plural, singular := meta.UnsafeGuessKindToResource(gvk.GroupVersion().WithKind(kindName))
	rm.Lock()
	delete(rm.dynamicRESTMapper, plural)
	delete(rm.dynamicRESTMapper, singular)
	rm.Unlock()
	return rm.updateCachedDynamicRESTMapper()
}

// Used to update local files saved on disk
func (rm *RESTMapperManager) updateCachedDynamicRESTMapper() error {
	if rm.storage == nil {
		return nil
	}
	rm.RLock()
	d, err := marshalDynamicRESTMapper(rm.dynamicRESTMapper)
	rm.RUnlock()
	if err != nil {
		return err
	}
	return rm.storage.Update(CacheDynamicRESTMapperKey, d)
}

// KindFor is used to find GVK based on GVR information.
// 1. return true means the GVR is a built-in resource in scheme.
// 2.1 return false and non-empty GVK means the GVR is custom resource
// 2.2 return false and empty GVK means the GVR is unknown resource.
func (rm *RESTMapperManager) KindFor(gvr schema.GroupVersionResource) (bool, schema.GroupVersionKind) {
	gvk, kindErr := rm.unsafeDefaultRESTMapper.KindFor(gvr)
	if kindErr != nil {
		gvk, kindErr = rm.dynamicKindFor(gvr)
		if kindErr != nil {
			return false, schema.GroupVersionKind{}
		}
		return false, gvk
	}
	return true, gvk
}

// DeleteKindFor is used to delete the GVK information related to the incoming gvr
func (rm *RESTMapperManager) DeleteKindFor(gvr schema.GroupVersionResource) error {
	isScheme, gvk := rm.KindFor(gvr)
	if !isScheme && !gvk.Empty() {
		return rm.deleteKind(gvk)
	}
	return nil
}

// UpdateKind is used to verify and add the GVK and GVR mapping relationships of new Custom Resource
func (rm *RESTMapperManager) UpdateKind(gvk schema.GroupVersionKind) error {
	kindName := strings.TrimSuffix(gvk.Kind, "List")
	gvk = gvk.GroupVersion().WithKind(kindName)
	plural, singular := meta.UnsafeGuessKindToResource(gvk.GroupVersion().WithKind(kindName))
	// If it is not a built-in resource and it is not stored in DynamicRESTMapper, add it to DynamicRESTMapper
	isScheme, t := rm.KindFor(singular)
	if !isScheme && t.Empty() {
		rm.Lock()
		rm.dynamicRESTMapper[singular] = gvk
		rm.dynamicRESTMapper[plural] = gvk
		rm.Unlock()
		return rm.updateCachedDynamicRESTMapper()
	}
	return nil
}

// ResetRESTMapper is used to clean up all cached GVR/GVK information in DynamicRESTMapper,
// and delete the corresponding file in the disk (cache-crd-restmapper.conf), it should be used carefully.
func (rm *RESTMapperManager) ResetRESTMapper() error {
	rm.dynamicRESTMapper = make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	err := rm.storage.DeleteCollection(CacheDynamicRESTMapperKey)
	if err != nil {
		return err
	}
	return nil
}

// marshalDynamicRESTMapper converts dynamicRESTMapper to the []byte format, which is used to save data to disk
func marshalDynamicRESTMapper(dynamicRESTMapper map[schema.GroupVersionResource]schema.GroupVersionKind) ([]byte, error) {
	cacheMapper := make(map[string]string, len(dynamicRESTMapper))
	for currResource, currKind := range dynamicRESTMapper {
		//key: Group/Version/Resource, value: Kind
		k := strings.Join([]string{currResource.Group, currResource.Version, currResource.Resource}, SepForGVR)
		cacheMapper[k] = currKind.Kind
	}
	return json.Marshal(cacheMapper)
}

// unmarshalDynamicRESTMapper converts bytes of data to map[schema.GroupVersionResource]schema.GroupVersionKind format, used to recover data from disk
func unmarshalDynamicRESTMapper(data []byte) map[schema.GroupVersionResource]schema.GroupVersionKind {
	dm := make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	cacheMapper := make(map[string]string)
	err := json.Unmarshal(data, &cacheMapper)
	if err != nil {
		klog.Errorf("failed to get cached CRDRESTMapper, %v", err)
	}

	for gvrString, kindString := range cacheMapper {
		localInfo := strings.Split(gvrString, SepForGVR)
		if len(localInfo) != 3 {
			klog.Errorf("This %s is not standardized, ignore this gvk", gvrString)
			continue
		}
		gvr := schema.GroupVersionResource{
			Group:    localInfo[0],
			Version:  localInfo[1],
			Resource: localInfo[2],
		}
		gvk := schema.GroupVersionKind{
			Group:   localInfo[0],
			Version: localInfo[1],
			Kind:    kindString,
		}
		dm[gvr] = gvk
	}
	return dm
}

// IsSchemeResource is used to determine whether gvr is a built-in resource
func IsSchemeResource(gvr schema.GroupVersionResource) bool {
	gvks, kindErr := unsafeSchemeRESTMapper.KindsFor(gvr)
	if kindErr != nil {
		return false
	}

	for i := range gvks {
		if gvks[i].Group == gvr.Group && gvks[i].Version == gvr.Version {
			return true
		}
	}

	return false
}
