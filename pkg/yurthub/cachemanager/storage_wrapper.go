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

package cachemanager

import (
	"bytes"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

// StorageWrapper is wrapper for storage.Store interface
// in order to handle serialize runtime object
type StorageWrapper interface {
	Name() string
	Create(key storage.Key, obj runtime.Object) error
	Delete(key storage.Key) error
	Get(key storage.Key) (runtime.Object, error)
	List(key storage.Key) ([]runtime.Object, error)
	Update(key storage.Key, obj runtime.Object, rv uint64) (runtime.Object, error)
	KeyFunc(info storage.KeyBuildInfo) (storage.Key, error)
	ListResourceKeysOfComponent(component string, gvr schema.GroupVersionResource) ([]storage.Key, error)
	ReplaceComponentList(component string, gvr schema.GroupVersionResource, namespace string, contents map[storage.Key]runtime.Object) error
	DeleteComponentResources(component string) error
	SaveClusterInfo(key storage.ClusterInfoKey, content []byte) error
	GetClusterInfo(key storage.ClusterInfoKey) ([]byte, error)
	GetStorage() storage.Store
	GetCacheResult() (int, string)
}

type storageWrapper struct {
	sync.RWMutex
	store             storage.Store
	errorKeys         *errorKeys
	backendSerializer runtime.Serializer
}

// NewStorageWrapper create a StorageWrapper object
func NewStorageWrapper(storage storage.Store) StorageWrapper {
	sw := &storageWrapper{
		store:             storage,
		errorKeys:         NewErrorKeys(),
		backendSerializer: json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{}),
	}
	sw.errorKeys.recover()
	return sw
}

func (sw *storageWrapper) Name() string {
	return sw.store.Name()
}

func (sw *storageWrapper) KeyFunc(info storage.KeyBuildInfo) (storage.Key, error) {
	return sw.store.KeyFunc(info)
}

func (sw *storageWrapper) GetStorage() storage.Store {
	return sw.store
}

func (sw *storageWrapper) GetCacheResult() (int, string) {
	return sw.errorKeys.length(), sw.errorKeys.aggregate()
}

// Create store runtime object into backend storage
// if obj is nil, the storage used to represent the key
// will be created. for example: for disk storage,
// a directory that indicates the key will be created.
func (sw *storageWrapper) Create(key storage.Key, obj runtime.Object) error {
	var buf bytes.Buffer
	if obj != nil {
		if err := sw.backendSerializer.Encode(obj, &buf); err != nil {
			sw.errorKeys.put(key.Key(), err.Error())
			klog.Errorf("could not encode object in create for %s, %v", key.Key(), err)
			return err
		}
	}

	if err := sw.store.Create(key, buf.Bytes()); err != nil {
		sw.errorKeys.put(key.Key(), err.Error())
		return err
	}

	sw.errorKeys.del(key.Key())
	return nil
}

// Delete remove runtime object that by specified key from backend storage
func (sw *storageWrapper) Delete(key storage.Key) error {
	err := sw.store.Delete(key)
	if err != nil {
		sw.errorKeys.put(key.Key(), fmt.Sprintf("failed to delete, %v", err.Error()))
		return err
	}
	sw.errorKeys.del(key.Key())
	return nil
}

// Get get the runtime object that specified by key from backend storage
func (sw *storageWrapper) Get(key storage.Key) (runtime.Object, error) {
	b, err := sw.store.Get(key)
	if err != nil {
		return nil, err
	} else if len(b) == 0 {
		return nil, nil
	}

	//get the gvk from json data
	gvk, err := json.DefaultMetaFactory.Interpret(b)
	if err != nil {
		return nil, err
	}

	var UnstructuredObj runtime.Object
	if scheme.Scheme.Recognizes(*gvk) {
		UnstructuredObj = nil
	} else {
		UnstructuredObj = new(unstructured.Unstructured)
	}
	obj, gvk, err := sw.backendSerializer.Decode(b, nil, UnstructuredObj)
	if err != nil {
		klog.Errorf("could not decode %v for %s, %v", gvk, key.Key(), err)
		return nil, err
	}

	return obj, nil
}

// ListKeys list all keys with key as prefix
func (sw *storageWrapper) ListResourceKeysOfComponent(component string, gvr schema.GroupVersionResource) ([]storage.Key, error) {
	return sw.store.ListResourceKeysOfComponent(component, gvr)
}

// List get all of runtime objects that specified by key as prefix
func (sw *storageWrapper) List(key storage.Key) ([]runtime.Object, error) {
	bb, err := sw.store.List(key)
	objects := make([]runtime.Object, 0, len(bb))
	if err != nil {
		klog.Errorf("could not list objects for %s, %v", key.Key(), err)
		return nil, err
	}
	if len(bb) == 0 {
		return objects, nil
	}
	//get the gvk from json data
	gvk, err := json.DefaultMetaFactory.Interpret(bb[0])
	if err != nil {
		return nil, err
	}
	var UnstructuredObj runtime.Object
	var recognized bool
	if scheme.Scheme.Recognizes(*gvk) {
		recognized = true
	}

	for i := range bb {
		if !recognized {
			UnstructuredObj = new(unstructured.Unstructured)
		}

		obj, gvk, err := sw.backendSerializer.Decode(bb[i], nil, UnstructuredObj)
		if err != nil {
			klog.Errorf("could not decode %v for %s, %v", gvk, key.Key(), err)
			continue
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// Update update runtime object in backend storage
func (sw *storageWrapper) Update(key storage.Key, obj runtime.Object, rv uint64) (runtime.Object, error) {
	var buf bytes.Buffer
	if err := sw.backendSerializer.Encode(obj, &buf); err != nil {
		sw.errorKeys.put(key.Key(), err.Error())
		klog.Errorf("could not encode object in update for %s, %v", key.Key(), err)
		return nil, err
	}

	if buf, err := sw.store.Update(key, buf.Bytes(), rv); err != nil {
		sw.errorKeys.put(key.Key(), err.Error())
		if err == storage.ErrUpdateConflict {
			obj, _, dErr := sw.backendSerializer.Decode(buf, nil, nil)
			if dErr != nil {
				return nil, fmt.Errorf("could not decode existing obj of key %s, %v", key.Key(), dErr)
			}
			return obj, err
		}
		return nil, err
	}
	sw.errorKeys.del(key.Key())
	return obj, nil
}

func (sw *storageWrapper) ReplaceComponentList(component string, gvr schema.GroupVersionResource, namespace string, objs map[storage.Key]runtime.Object) error {
	var buf bytes.Buffer
	key, _ := sw.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Group:     gvr.Group,
		Version:   gvr.Version,
		Resources: gvr.Resource,
		Namespace: namespace,
	})
	contents := make(map[storage.Key][]byte, len(objs))
	for key, obj := range objs {
		if err := sw.backendSerializer.Encode(obj, &buf); err != nil {
			sw.errorKeys.put(key.Key(), err.Error())
			klog.Errorf("could not encode object in update for %s, %v", key.Key(), err)
			return err
		}
		contents[key] = make([]byte, len(buf.Bytes()))
		copy(contents[key], buf.Bytes())
		buf.Reset()
	}

	err := sw.store.ReplaceComponentList(component, gvr, namespace, contents)
	if err != nil {
		sw.errorKeys.put(key.Key(), err.Error())
		return err
	}
	sw.errorKeys.del(key.Key())
	return nil
}

// DeleteCollection will delete all objects under rootKey
func (sw *storageWrapper) DeleteComponentResources(component string) error {
	err := sw.store.DeleteComponentResources(component)
	if err != nil {
		sw.errorKeys.put(component, fmt.Sprintf("failed to delete, %v", err.Error()))
		return err
	}
	sw.errorKeys.del(component)
	return nil
}

func (sw *storageWrapper) SaveClusterInfo(key storage.ClusterInfoKey, content []byte) error {
	err := sw.store.SaveClusterInfo(key, content)
	if err != nil {
		sw.errorKeys.put(string(key.ClusterInfoType), fmt.Sprintf("failed to store cluster info, %v", err.Error()))
		return err
	}
	sw.errorKeys.del(string(key.ClusterInfoType))
	return nil
}

func (sw *storageWrapper) GetClusterInfo(key storage.ClusterInfoKey) ([]byte, error) {
	return sw.store.GetClusterInfo(key)
}
