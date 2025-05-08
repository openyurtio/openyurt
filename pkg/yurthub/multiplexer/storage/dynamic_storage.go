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
	"math/rand"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/rest"
)

func init() {
	metav1.AddToGroupVersion(parameterScheme, versionV1)
}

type dynamicStorage struct {
	client   rest.Interface
	resource string
}

func newDynamicStorage(client rest.Interface, resource string) *dynamicStorage {
	return &dynamicStorage{
		client:   client,
		resource: resource,
	}
}

func (rs *dynamicStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	listOpts := &metav1.ListOptions{
		Limit:                opts.Predicate.Limit,
		Continue:             opts.Predicate.Continue,
		ResourceVersionMatch: opts.ResourceVersionMatch,
		ResourceVersion:      opts.ResourceVersion,
	}

	result := rs.client.Get().Resource(rs.resource).SpecificallyVersionedParams(listOpts, dynamicParameterCodec, versionV1).Do(ctx)
	if err := result.Error(); err != nil {
		return err
	}
	retBytes, err := result.Raw()
	if err != nil {
		return err
	}
	listObj, ok := listObj.(*unstructured.UnstructuredList)
	if !ok {
		return errors.New("listObj is not of type *unstructured.UnstructuredList")
	}
	if err := runtime.DecodeInto(unstructured.UnstructuredJSONScheme, retBytes, listObj); err != nil {
		return err
	}

	return nil
}

func (rs *dynamicStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	timeoutSeconds := int64(minWatchTimeout.Seconds() * (rand.Float64() + 1.0))

	listOpts := &metav1.ListOptions{
		ResourceVersion:     opts.ResourceVersion,
		Watch:               true,
		TimeoutSeconds:      &timeoutSeconds,
		AllowWatchBookmarks: true,
	}
	w, err := rs.client.Get().Resource(rs.resource).SpecificallyVersionedParams(listOpts, dynamicParameterCodec, versionV1).Watch(ctx)

	return w, err
}

func (rs *dynamicStorage) Versioner() storage.Versioner {
	return nil
}

func (rs *dynamicStorage) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	return ErrNoSupport
}

func (rs *dynamicStorage) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, cachedExistingObject runtime.Object) error {
	return ErrNoSupport
}

func (rs *dynamicStorage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	return ErrNoSupport
}

func (rs *dynamicStorage) GuaranteedUpdate(
	ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	return ErrNoSupport
}

func (rs *dynamicStorage) Count(key string) (int64, error) {
	return 0, ErrNoSupport
}

func (rs *dynamicStorage) ReadinessCheck() error {
	return ErrNoSupport
}

func (rs *dynamicStorage) RequestWatchProgress(ctx context.Context) error {
	return ErrNoSupport
}

type Interface interface {
	Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error)
	GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error
	ReadinessCheck() error
}

type ResourceCacheConfig struct {
	KeyFunc      func(runtime.Object) (string, error)
	NewFunc      func() runtime.Object
	NewListFunc  func() runtime.Object
	GetAttrsFunc storage.AttrFunc
}
