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

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

// StorageWrapper is wrapper for storage.Store interface
// in order to handle serialize runtime object
type StorageWrapper interface {
	Store
	SaveClusterInfo(key ClusterInfoKey, content []byte) error
	GetClusterInfo(key ClusterInfoKey) ([]byte, error)
	GetStorage() Store
}

type storageWrapper struct {
	sync.RWMutex
	store             Store
	backendSerializer runtime.Serializer
	queue             Interface
}

// NewStorageWrapper create a StorageWrapper object
func NewStorageWrapper(storage Store, queue Interface) StorageWrapper {
	sw := &storageWrapper{
		store:             storage,
		backendSerializer: json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{}),
		queue:             queue,
	}
	return sw
}

func (sw *storageWrapper) Name() string {
	return sw.store.Name()
}

func (sw *storageWrapper) KeyFunc(info KeyBuildInfo) (Key, error) {
	return sw.store.KeyFunc(info)
}

func (sw *storageWrapper) GetStorage() Store {
	return sw.store
}

// Create store runtime object into backend storage
// if obj is nil, the storage used to represent the key
// will be created. for example: for disk storage,
// a directory that indicates the key will be created.
func (sw *storageWrapper) Create(key Key, obj runtime.Object) error {
	item := Item{
		Key:    key,
		Object: obj,
		Verb:   "create",
	}
	sw.queue.Add(item)
	return nil
}

// Delete remove runtime object that by specified key from backend storage
func (sw *storageWrapper) Delete(key Key) error {
	item := Item{
		Key:  key,
		Verb: "delete",
	}
	sw.queue.Add(item)
	return nil
}

// Get get the runtime object that specified by key from backend storage
func (sw *storageWrapper) Get(key Key) (runtime.Object, error) {
	obj, err := sw.store.Get(key)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// ListKeys list all keys with key as prefix
func (sw *storageWrapper) ListKeys(key Key) ([]Key, error) {
	return sw.store.ListKeys(key)
}

// List get all of runtime objects that specified by key as prefix
func (sw *storageWrapper) List(key Key) ([]runtime.Object, error) {
	objects, err := sw.store.List(key)
	if err != nil {
		return nil, err
	}
	return objects, nil
}

// Update update runtime object in backend storage
func (sw *storageWrapper) Update(key Key, obj runtime.Object, rv uint64) (runtime.Object, error) {
	item := Item{
		Key:             key,
		Object:          obj,
		ResourceVersion: rv,
		Verb:            "Update",
	}
	sw.queue.Add(item)
	return obj, nil
}

func (sw *storageWrapper) Replace(key Key, objs map[Key]runtime.Object) error {
	var items []Item
	for key, obj := range objs {
		items = append(items, Item{
			Key:    key,
			Object: obj,
			Verb:   "list",
		})
	}
	sw.queue.Replace(items)
	return nil
}

func (sw *storageWrapper) SaveClusterInfo(key ClusterInfoKey, content []byte) error {
	return sw.store.SaveClusterInfo(key, content)
}

func (sw *storageWrapper) GetClusterInfo(key ClusterInfoKey) ([]byte, error) {
	return sw.store.GetClusterInfo(key)
}
