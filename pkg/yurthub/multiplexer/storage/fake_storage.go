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

package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

type CommonFakeStorage struct {
}

func (fs *CommonFakeStorage) Versioner() storage.Versioner {
	return nil
}

func (fs *CommonFakeStorage) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	return nil
}

func (fs *CommonFakeStorage) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, cachedExistingObject runtime.Object) error {
	return nil
}

func (fs *CommonFakeStorage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	return nil
}

func (fs *CommonFakeStorage) GuaranteedUpdate(
	ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	return nil
}

func (fs *CommonFakeStorage) Count(key string) (int64, error) {
	return 0, nil
}

func (fs *CommonFakeStorage) ReadinessCheck() error {
	return nil
}

func (fs *CommonFakeStorage) RequestWatchProgress(ctx context.Context) error {
	return nil
}

type FakeServiceStorage struct {
	*CommonFakeStorage
	items   []v1.Service
	watcher *watch.FakeWatcher
}

func NewFakeServiceStorage(items []v1.Service) *FakeServiceStorage {
	return &FakeServiceStorage{
		CommonFakeStorage: &CommonFakeStorage{},
		items:             items,
		watcher:           watch.NewFake(),
	}
}

func (fs *FakeServiceStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	serviceList := listObj.(*v1.ServiceList)
	serviceList.ListMeta = metav1.ListMeta{
		ResourceVersion: "100",
	}
	serviceList.Items = fs.items
	return nil
}

func (fs *FakeServiceStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	return fs.watcher, nil
}

func (fs *FakeServiceStorage) AddWatchObject(svc *v1.Service) {
	svc.ResourceVersion = "101"
	fs.watcher.Add(svc)
}

type FakeEndpointSliceStorage struct {
	*CommonFakeStorage
	items   []discovery.EndpointSlice
	watcher *watch.FakeWatcher
}

func NewFakeEndpointSliceStorage(items []discovery.EndpointSlice) *FakeEndpointSliceStorage {
	return &FakeEndpointSliceStorage{
		CommonFakeStorage: &CommonFakeStorage{},
		items:             items,
		watcher:           watch.NewFake(),
	}
}

func (fs *FakeEndpointSliceStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	epsList := listObj.(*discovery.EndpointSliceList)
	epsList.ListMeta = metav1.ListMeta{
		ResourceVersion: "100",
	}

	for _, item := range fs.items {
		itemKey := fmt.Sprintf("/%s/%s", item.Namespace, item.Name)
		if strings.HasPrefix(itemKey, key) {
			epsList.Items = append(epsList.Items, item)
		}
	}
	return nil
}

func (fs *FakeEndpointSliceStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	return fs.watcher, nil
}

func (fs *FakeEndpointSliceStorage) AddWatchObject(eps *discovery.EndpointSlice) {
	eps.ResourceVersion = "101"
	fs.watcher.Add(eps)
}

type FakeFooStorage struct {
	*CommonFakeStorage
	items   []unstructured.Unstructured
	watcher *watch.FakeWatcher
	lock    sync.Mutex
}

func NewFakeFooStorage(items []unstructured.Unstructured) *FakeFooStorage {
	return &FakeFooStorage{
		CommonFakeStorage: &CommonFakeStorage{},
		items:             items,
		watcher:           watch.NewFake(),
	}
}

func (f *FakeFooStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	list, ok := listObj.(*unstructured.UnstructuredList)
	list.SetResourceVersion("100")
	if !ok {
		return fmt.Errorf("unexpected list type: %T", listObj)
	}
	list.Object["metadata"] = map[string]interface{}{
		"resourceVersion": "100",
	}
	for _, item := range f.items {
		itemKey := f.getResourceKey(item)
		if strings.HasPrefix(itemKey, key) {
			list.Items = append(list.Items, item)
		}
	}
	return nil
}

func (f *FakeFooStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	return f.watcher, nil
}

func (f *FakeFooStorage) AddWatchObject(foo *unstructured.Unstructured) {
	foo.SetResourceVersion("101")
	f.watcher.Add(foo)
}

func (f *FakeFooStorage) getResourceKey(foo unstructured.Unstructured) string {
	namespace := foo.GetNamespace()
	name := foo.GetName()
	if namespace == "" {
		namespace = "default"
	}
	return fmt.Sprintf("%s/%s", namespace, name)
}
