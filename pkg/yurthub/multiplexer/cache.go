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

package multiplexer

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/cacher"
	"k8s.io/client-go/kubernetes/scheme"
)

type Interface interface {
	Watch(ctx context.Context, key string, opts kstorage.ListOptions) (watch.Interface, error)
	GetList(ctx context.Context, key string, opts kstorage.ListOptions, listObj runtime.Object) error
	ReadinessCheck() error
}

type ResourceCacheConfig struct {
	KeyFunc      func(runtime.Object) (string, error)
	NewFunc      func() runtime.Object
	NewListFunc  func() runtime.Object
	GetAttrsFunc kstorage.AttrFunc
}

func NewResourceCache(
	s kstorage.Interface,
	resource *schema.GroupVersionResource,
	config *ResourceCacheConfig, isCRD bool) (Interface, func(), error) {
	cacheConfig := cacher.Config{
		Storage:       s,
		Versioner:     kstorage.APIObjectVersioner{},
		GroupResource: resource.GroupResource(),
		KeyFunc:       config.KeyFunc,
		NewFunc:       config.NewFunc,
		NewListFunc:   config.NewListFunc,
		GetAttrsFunc:  config.GetAttrsFunc,
	}
	gv := resource.GroupVersion()
	var codec runtime.Codec
	if !isCRD {
		codec = scheme.Codecs.LegacyCodec(gv)
	} else {
		codec = unstructured.UnstructuredJSONScheme

	}
	cacheConfig.Codec = codec
	cacher, err := cacher.NewCacherFromConfig(cacheConfig)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to new cacher from config, error: %v", err)
	}

	var once sync.Once
	destroyFunc := func() {
		once.Do(func() {
			cacher.Stop()
		})
	}

	return cacher, destroyFunc, nil
}
