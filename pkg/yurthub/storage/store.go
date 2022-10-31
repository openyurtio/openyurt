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

package storage

type ClusterInfoKey struct {
	ClusterInfoType
	UrlPath string
}

type ClusterInfoType string

const (
	Version          ClusterInfoType = "version"
	APIsInfo         ClusterInfoType = "apis"
	APIResourcesInfo ClusterInfoType = "api-resources"
	Unknown          ClusterInfoType = "unknown"
)

// Store is an interface for caching data into store
type Store interface {
	// Name will return the name of this store.
	Name() string
	clusterInfoRelatedInterface
	objectRelatedInterface
	componentRelatedInterface
}

// clusterInfoRelatedInterface contains functions for manipulating cluster info cache in the storage.
type clusterInfoRelatedInterface interface {
	// SaveClusterInfo will save content of cluster info into storage.
	// If the content has already existed in the storage, it will be overwritten with content.
	SaveClusterInfo(key ClusterInfoKey, content []byte) error
	// GetClusterInfo will get the cluster info of clusterInfoType from storage.
	// If the cluster info is not found in the storage, return ErrStorageNotFound.
	GetClusterInfo(key ClusterInfoKey) ([]byte, error)
}

// objectRelatedInterface contains functions for manipulating resource objects in the format of key-value
// in the storage.
// Note:
// The description for each function in this interface only contains
// the interface-related error, which means other errors are also possibly returned,
// such as errors when reading/opening files.
type objectRelatedInterface interface {
	// Create will create content of key in the store.
	// The key must indicate a specific resource.
	// If key is empty, ErrKeyIsEmpty will be returned.
	// If this key has already existed in this store, ErrKeyExists will be returned.
	Create(key Key, content []byte) error

	// Delete will delete the content of key in the store.
	// The key must indicate a specific resource.
	// If key is empty, ErrKeyIsEmpty will be returned.
	Delete(key Key) error

	// Get will get the content of key from the store.
	// The key must indicate a specific resource.
	// If key is empty, ErrKeyIsEmpty will be returned.
	// If this key does not exist in this store, ErrStorageNotFound will be returned.
	Get(key Key) ([]byte, error)

	// List will retrieve all contents whose keys have the prefix of rootKey.
	// If rootKey is empty, ErrKeyIsEmpty will be returned.
	// If the rootKey does not exist in the store, ErrStorageNotFound will be returned.
	// If the rootKey exists in the store but no keys has the prefix of rootKey,
	// an empty slice of content will be returned.
	List(rootKey Key) ([][]byte, error)

	// Update will try to update key in store with passed-in contents. Only when
	// the rv of passed-in contents is fresher than what is in the store, the Update will happen.
	// The content of key after Update is completed will be returned.
	// The key must indicate a specific resource.
	// If key is empty, ErrKeyIsEmpty will be returned.
	// If the key does not exist in the store, ErrStorageNotFound will be returned.
	// If force is not set and the rv is staler than what is in the store, ErrUpdateConflict will be returned.
	Update(key Key, contents []byte, rv uint64) ([]byte, error)

	// KeyFunc will generate the key used by this store.
	// info contains necessary info to generate the key for the object. How to use this info
	// to generate the key depends on the implementation of storage.
	KeyFunc(info KeyBuildInfo) (Key, error)

	// TODO: RootKeyFunc()
	// decouple key with root key
}

// componentRelatedInterface contains functions for manipulating objects in the storage based on the component,
// such as getting keys of all objects cached for some component. The difference between it and objectRelatedInterface is
// it doesn't need object key and only provide limited function for special usage, such as gc.
// TODO: reconsider the interface, if the store should be conscious of the component.
type componentRelatedInterface interface {
	// ListResourceKeysOfComponent will get all keys of resource of component.
	// If component is Empty, ErrEmptyComponent will be returned.
	// If resource is Empty, ErrEmptyResource will be returned.
	// If the cache of component can not be found or the resource has not been cached, return ErrStorageNotFound.
	ListResourceKeysOfComponent(component string, resource string) ([]Key, error)

	// ReplaceComponentList will replace all cached objs of resource associated with the component with the passed-in contents.
	// If the cached objs does not exist, it will use contents to build the cache. This function is used by CacheManager to
	// save list objects. It works like using the new list objects which are passed in as contents arguments to replace
	// relative old ones.
	// If namespace is provided, only objs in this namespace will be replaced.
	// If namespace is not provided, objs of all namespaces will be replaced with provided contents.
	// If component is empty, ErrEmptyComponent will be returned.
	// If resource is empty, ErrEmptyResource will be returned.
	// If contents is empty, only the base dir of them will be created. Refer to #258.
	// If some contents are not the specified the resource, ErrInvalidContent will be returned.
	// If the specified resource does not exist in the store, it will be created with passed-in contents.
	ReplaceComponentList(component string, resource string, namespace string, contents map[Key][]byte) error

	// DeleteComponentResources will delete all resources associated with the component.
	// If component is Empty, ErrEmptyComponent will be returned.
	DeleteComponentResources(component string) error
}
