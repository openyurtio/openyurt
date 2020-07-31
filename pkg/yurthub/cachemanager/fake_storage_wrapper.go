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
	"os"
	"strings"

	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"github.com/alibaba/openyurt/pkg/yurthub/storage/fake"

	"k8s.io/apimachinery/pkg/runtime"
)

type fakeStorageWrapper struct {
	s    storage.Store
	data map[string]runtime.Object
}

// NewFakeStorageWrapper new fake storage wrapper
func NewFakeStorageWrapper() StorageWrapper {
	s, _ := fake.NewFakeStorage()
	return &fakeStorageWrapper{
		s:    s,
		data: make(map[string]runtime.Object),
	}
}

func (fsw *fakeStorageWrapper) Create(key string, obj runtime.Object) error {
	if fsw.data == nil {
		fsw.data = make(map[string]runtime.Object)
	}
	fsw.data[key] = obj

	return nil
}

func (fsw *fakeStorageWrapper) Delete(key string) error {
	delete(fsw.data, key)

	return nil
}

func (fsw *fakeStorageWrapper) Get(key string) (runtime.Object, error) {
	obj, ok := fsw.data[key]
	if ok {
		return obj, nil
	}

	return nil, os.ErrNotExist
}

func (fsw *fakeStorageWrapper) ListKeys(key string) ([]string, error) {
	keys := make([]string, 0, len(fsw.data))
	for k := range fsw.data {
		keys = append(keys, k)
	}

	return keys, nil
}

func (fsw *fakeStorageWrapper) List(key string) ([]runtime.Object, error) {
	objs := make([]runtime.Object, 0, len(fsw.data))
	for k, obj := range fsw.data {
		if strings.HasPrefix(k, key) {
			objs = append(objs, obj)
		}
	}

	return objs, nil
}

func (fsw *fakeStorageWrapper) Update(key string, obj runtime.Object) error {
	if fsw.data == nil {
		fsw.data = make(map[string]runtime.Object)
	}
	fsw.data[key] = obj

	return nil
}

func (fsw *fakeStorageWrapper) GetRaw(key string) ([]byte, error) {
	return fsw.s.Get(key)
}

func (fsw *fakeStorageWrapper) UpdateRaw(key string, contents []byte) error {
	return fsw.s.Update(key, contents)
}
