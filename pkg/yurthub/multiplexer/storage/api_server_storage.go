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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const minWatchRequestSeconds = 300

var ErrNoSupport = errors.New("Don't Support Method ")

type apiServerStorage struct {
	restClient rest.Interface
	resource   string
}

func NewStorage(restClient rest.Interface, resource string) storage.Interface {
	return &apiServerStorage{
		restClient: restClient,
		resource:   resource,
	}
}

func (rs *apiServerStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	listOpts := &metav1.ListOptions{
		Limit:                opts.Predicate.Limit,
		Continue:             opts.Predicate.Continue,
		ResourceVersionMatch: opts.ResourceVersionMatch,
		ResourceVersion:      opts.ResourceVersion,
	}

	return rs.restClient.Get().Resource(rs.resource).VersionedParams(listOpts, scheme.ParameterCodec).Do(ctx).Into(listObj)
}

func (rs *apiServerStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	timeoutSeconds := int64(float64(minWatchRequestSeconds) * (rand.Float64() + 1.0))

	listOpts := &metav1.ListOptions{
		ResourceVersion: opts.ResourceVersion,
		Watch:           true,
		TimeoutSeconds:  &timeoutSeconds,
	}

	w, err := rs.restClient.Get().Resource(rs.resource).VersionedParams(listOpts, scheme.ParameterCodec).Watch(ctx)

	return w, err
}

func (rs *apiServerStorage) Versioner() storage.Versioner {
	return nil
}

func (rs *apiServerStorage) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	return ErrNoSupport
}

func (rs *apiServerStorage) Delete(
	ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions,
	validateDeletion storage.ValidateObjectFunc, cachedExistingObject runtime.Object) error {
	return ErrNoSupport
}

func (rs *apiServerStorage) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	return ErrNoSupport
}

func (rs *apiServerStorage) GuaranteedUpdate(
	ctx context.Context, key string, destination runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	return ErrNoSupport
}

func (rs *apiServerStorage) Count(key string) (int64, error) {
	return 0, ErrNoSupport
}

func (rs *apiServerStorage) RequestWatchProgress(ctx context.Context) error {
	return ErrNoSupport
}
