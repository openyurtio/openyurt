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

package fake

import "github.com/alibaba/openyurt/pkg/yurthub/storage"

type fakeStorage struct {
	data map[string]string
}

// NewFakeStorage creates a fake storage
func NewFakeStorage() (storage.Store, error) {
	return &fakeStorage{
		data: make(map[string]string),
	}, nil
}

func (fs *fakeStorage) Create(key string, contents []byte) error {
	fs.data[key] = string(contents)
	return nil
}

func (fs *fakeStorage) Delete(key string) error {
	delete(fs.data, key)
	return nil
}

func (fs *fakeStorage) Get(key string) ([]byte, error) {
	s, ok := fs.data[key]
	if ok {
		return []byte(s), nil
	}
	return []byte{}, nil
}

func (fs *fakeStorage) ListKeys(key string) ([]string, error) {
	keys := make([]string, 0, len(fs.data))
	for k := range fs.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (fs *fakeStorage) List(key string) ([][]byte, error) {
	bb := make([][]byte, 0, len(fs.data))
	for _, v := range fs.data {
		bb = append(bb, []byte(v))
	}
	return bb, nil
}

func (fs *fakeStorage) Update(key string, contents []byte) error {
	fs.data[key] = string(contents)
	return nil
}
