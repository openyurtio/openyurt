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

// Store is an interface for caching data into store
//
// Note:
// The description for each function in this interface only contains
// the interface-related error, which means other errors are also possibly returned,
// such as errors when reading/opening files.
type Store interface {
	// Name will return the name of this store.
	Name() string

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

	// TODO: if cannot update for rv is not fresh enough, the current obj bytes should be return
	//
	// Update will try to update key in store with passed-in contents. Only when
	// the rv of passed-in contents is fresher than what is in the store, the Update will happen.
	// The content of key after Update is completed will be returned.
	// If force is set as true, rv will be ignored, using the passed-in contents to
	// replace what is in the store.
	// The key must indicate a specific resource.
	// If key is empty, ErrKeyIsEmpty will be returned.
	// If the key does not exist in the store, ErrStorageNotFound will be returned.
	// If force is not set and the rv is staler than what is in the store, ErrUpdateConflict will be returned.
	Update(key Key, contents []byte, rv uint64) ([]byte, error)

	// KeyFunc will generate the key of the object used by this store.
	// info contains necessary infos used to generate the key. How to use it
	// depends on the implementation of the storage.
	KeyFunc(info KeyBuildInfo) (Key, error)

	// TODO: RootKeyFunc()
	// decouple key with root key

	componentRelatedInterface
}

// TODO: reconsider the interface, if the store should be conscious of the component.
type componentRelatedInterface interface {
	// ListResourceKeysOfComponent will get all keys of resource of component.
	// If component is Empty, ErrEmptyComponent will be returned.
	// If resource is Empty, ErrEmptyResource will be returned.
	// If the component can not be recognized or the resource has not been cached, return ErrStorageNotFound.
	ListResourceKeysOfComponent(component string, resource string) ([]Key, error)

	// ReplaceComponentList will replace all cached objs of resource associated with the component with the passed-in contents.
	// If the cached objs does not exist, it will use contents to build the cache. The selector parameter indicates the list
	// selector it uses. This function is used by CacheManager to save list objects. It works like using the new list objects,
	// passed in with contents, to replace relative old ones.
	// If namespace is provided, only objs in this namespace will be replaced.
	// If component is Empty, ErrEmptyComponent will be returned.
	// If resource is Empty, ErrEmptyResource will be returned.
	// If the passed-in selector for this resource of this component is different from the previous one,
	// the replace will fail and return error.
	// If some contents are not the specified the resource, ErrInvalidContent will be returned.
	// If the specified resource does not exist in the store, it will be created with passed-in contents.
	ReplaceComponentList(component string, resource string, namespace string, selector string, contents map[Key][]byte) error

	// DeleteComponentResources will delete all resources associated with the component.
	// If component is Empty, ErrEmptyComponent will be returned.
	DeleteComponentResources(component string) error
}
