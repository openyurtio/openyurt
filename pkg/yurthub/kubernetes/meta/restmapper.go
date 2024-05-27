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
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
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

	specifiedResources = []string{
		"gateway",
	}
)

// RESTMapperManager is responsible for managing different kind of RESTMapper
type RESTMapperManager struct {
	sync.RWMutex
	cachedFilePath string
	storage        fs.FileSystemOperator
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

func NewRESTMapperManager(baseDir string) (*RESTMapperManager, error) {
	dm := make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	cachedFilePath := filepath.Join(baseDir, CacheDynamicRESTMapperKey)
	// Recover the mapping relationship between GVR and GVK from the hard disk
	storage := fs.FileSystemOperator{}
	b, err := storage.Read(cachedFilePath)
	if err == fs.ErrNotExists {
		dm = make(map[schema.GroupVersionResource]schema.GroupVersionKind)
		err = storage.CreateFile(filepath.Join(baseDir, CacheDynamicRESTMapperKey), []byte{})
		if err != nil {
			return nil, fmt.Errorf("could not init dynamic RESTMapper file at %s, %v", cachedFilePath, err)
		}
		klog.Infof("initialize an empty DynamicRESTMapper")
	} else if err != nil {
		return nil, fmt.Errorf("could not read existing RESTMapper file at %s, %v", cachedFilePath, err)
	}

	if len(b) != 0 {
		dm, err = unmarshalDynamicRESTMapper(b)
		if err != nil {
			return nil, fmt.Errorf("unrecognized content in %s for err %v, initialization of RESTMapper failed", cachedFilePath, err)
		}
		klog.Infof("reset DynamicRESTMapper to %v", dm)
	}

	return &RESTMapperManager{
		unsafeDefaultRESTMapper: unsafeSchemeRESTMapper,
		dynamicRESTMapper:       dm,
		cachedFilePath:          cachedFilePath,
		storage:                 storage,
	}, nil
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
	plural, singular := specifiedKindToResource(gvk.GroupVersion().WithKind(kindName))
	rm.Lock()
	delete(rm.dynamicRESTMapper, plural)
	delete(rm.dynamicRESTMapper, singular)
	rm.Unlock()
	return rm.updateCachedDynamicRESTMapper()
}

// Used to update local files saved on disk
func (rm *RESTMapperManager) updateCachedDynamicRESTMapper() error {
	rm.RLock()
	d, err := marshalDynamicRESTMapper(rm.dynamicRESTMapper)
	rm.RUnlock()
	if err != nil {
		return err
	}
	err = rm.storage.Write(rm.cachedFilePath, d)
	if err != nil {
		return fmt.Errorf("could not update cached dynamic RESTMapper, %v", err)
	}
	return nil
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
	plural, singular := specifiedKindToResource(gvk.GroupVersion().WithKind(kindName))
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
	return rm.storage.DeleteFile(rm.cachedFilePath)
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
func unmarshalDynamicRESTMapper(data []byte) (map[schema.GroupVersionResource]schema.GroupVersionKind, error) {
	dm := make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	cacheMapper := make(map[string]string)
	err := json.Unmarshal(data, &cacheMapper)
	if err != nil {
		return nil, fmt.Errorf("could not get cached CRDRESTMapper, %v", err)
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
	return dm, nil
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

// specifiedKindToResource converts Kind to a resource name.
// Broken. This method only "sort of" works when used outside of this package.  It assumes that Kinds and Resources match
// and they aren't guaranteed to do so.
func specifiedKindToResource(kind schema.GroupVersionKind) ( /*plural*/ schema.GroupVersionResource /*singular*/, schema.GroupVersionResource) {
	kindName := kind.Kind
	if len(kindName) == 0 {
		return schema.GroupVersionResource{}, schema.GroupVersionResource{}
	}
	singularName := strings.ToLower(kindName)
	singular := kind.GroupVersion().WithResource(singularName)

	for _, skip := range specifiedResources {
		if strings.HasSuffix(singularName, skip) {
			return kind.GroupVersion().WithResource(singularName + "s"), singular
		}
	}

	return meta.UnsafeGuessKindToResource(kind)
}
