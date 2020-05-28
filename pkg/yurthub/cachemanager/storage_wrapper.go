package cachemanager

import (
	"bytes"
	"sync"

	"github.com/alibaba/openyurt/pkg/yurthub/util"

	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
)

// StorageWrapper is wrapper for storage.Store interface
// in order to handle serialize runtime object
type StorageWrapper interface {
	Create(key string, obj runtime.Object) error
	Delete(key string) error
	Get(key string) (runtime.Object, error)
	ListKeys(key string) ([]string, error)
	List(key string) ([]runtime.Object, error)
	Update(key string, obj runtime.Object) error
	GetRaw(key string) ([]byte, error)
	UpdateRaw(key string, contents []byte) error
}

type storageWrapper struct {
	sync.RWMutex
	store             storage.Store
	backendSerializer runtime.Serializer
	cache             map[string]runtime.Object
}

// NewStorageWrapper create a StorageWrapper object
func NewStorageWrapper(storage storage.Store) StorageWrapper {
	return &storageWrapper{
		store:             storage,
		backendSerializer: json.NewSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, false),
		cache:             make(map[string]runtime.Object),
	}
}

// Create store runtime object into backend storage
func (sw *storageWrapper) Create(key string, obj runtime.Object) error {
	var buf bytes.Buffer
	if err := sw.backendSerializer.Encode(obj, &buf); err != nil {
		klog.Errorf("failed to encode object in create for %s, %v", key, err)
		return err
	}

	if err := sw.store.Create(key, buf.Bytes()); err != nil {
		return err
	}

	if isCacheKey(key) {
		sw.Lock()
		sw.cache[key] = obj
		sw.Unlock()
	}

	return nil
}

// Delete remove runtime object that by specified key from backend storage
func (sw *storageWrapper) Delete(key string) error {
	if err := sw.store.Delete(key); err != nil {
		return err
	}

	if isCacheKey(key) {
		sw.Lock()
		delete(sw.cache, key)
		sw.Unlock()
	}

	return nil
}

// Get get the runtime object that specified by key from backend storage
func (sw *storageWrapper) Get(key string) (runtime.Object, error) {
	cachedKey := isCacheKey(key)
	if cachedKey {
		sw.RLock()
		cachedObject, ok := sw.cache[key]
		sw.RUnlock()
		if ok && cachedObject != nil {
			return cachedObject, nil
		}
	}

	b, err := sw.store.Get(key)
	if err != nil {
		klog.Errorf("could not get object for %s, %v", key, err)
		return nil, err
	} else if len(b) == 0 {
		return nil, nil
	}

	obj, gvk, err := sw.backendSerializer.Decode(b, nil, nil)
	if err != nil {
		klog.Errorf("could not decode %v for %s, %v", gvk, key, err)
		return nil, err
	}

	if cachedKey {
		sw.Lock()
		sw.cache[key] = obj
		sw.Unlock()
	}
	return obj, nil
}

// ListKeys list all keys with key as prefix
func (sw *storageWrapper) ListKeys(key string) ([]string, error) {
	return sw.store.ListKeys(key)
}

// List get all of runtime objects that specified by key as prefix
func (sw *storageWrapper) List(key string) ([]runtime.Object, error) {
	objects := make([]runtime.Object, 0)
	bb, err := sw.store.List(key)
	if err != nil {
		klog.Errorf("could not list objects for %s, %v", key, err)
		return nil, err
	} else if len(bb) == 0 {
		return objects, nil
	}

	for i := range bb {
		obj, gvk, err := sw.backendSerializer.Decode(bb[i], nil, nil)
		if err != nil {
			klog.Errorf("could not decode %v for %s, %v", gvk, key, err)
			continue
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// Update update runtime object in backend storage
func (sw *storageWrapper) Update(key string, obj runtime.Object) error {
	var buf bytes.Buffer
	if err := sw.backendSerializer.Encode(obj, &buf); err != nil {
		klog.Errorf("failed to encode object in update for %s, %v", key, err)
		return err
	}

	if err := sw.store.Update(key, buf.Bytes()); err != nil {
		return err
	}

	if isCacheKey(key) {
		sw.Lock()
		sw.cache[key] = obj
		sw.Unlock()
	}

	return nil
}

// GetRaw get byte data for specified key
func (sw *storageWrapper) GetRaw(key string) ([]byte, error) {
	return sw.store.Get(key)
}

// UpdateRaw update contents(byte date) for specified key
func (sw *storageWrapper) UpdateRaw(key string, contents []byte) error {
	return sw.store.Update(key, contents)
}

// isCacheKey verify runtime object is cached for specified key.
// in order to accelerate kubelet get node and lease object, we cache them
func isCacheKey(key string) bool {
	comp, resource, _, _ := util.SplitKey(key)
	switch {
	case comp == "kubelet" && resource == "nodes":
		return true
	case comp == "kubelet" && resource == "leases":
		return true
	}

	return false
}
