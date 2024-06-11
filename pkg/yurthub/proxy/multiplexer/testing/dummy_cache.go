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

package testing

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

type DummyCache struct {
	WatchFunc   func(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error)
	GetListFunc func(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error
	Error       error
}

func (dc *DummyCache) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	if dc.WatchFunc != nil {
		return dc.WatchFunc(ctx, key, opts)
	}
	return nil, dc.Error
}

func (dc *DummyCache) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	if dc.GetListFunc != nil {
		return dc.GetListFunc(ctx, key, opts, listObj)
	}

	return dc.Error
}
