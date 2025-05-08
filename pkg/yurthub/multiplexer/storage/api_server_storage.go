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
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const minWatchTimeout = 5 * time.Minute

var ErrNoSupport = errors.New("Don't Support Method ")

func init() {
	metav1.AddToGroupVersion(parameterScheme, versionV1)
}

type apiServerStorage struct {
	restClient rest.Interface
	resource   string
	isCRD      bool
}

func NewStorage(restClient rest.Interface, resource string, isCRD bool) storage.Interface {
	return &apiServerStorage{
		restClient: restClient,
		resource:   resource,
		isCRD:      isCRD,
	}
}

func (rs *apiServerStorage) GetList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	listOpts := &metav1.ListOptions{
		Limit:                opts.Predicate.Limit,
		Continue:             opts.Predicate.Continue,
		ResourceVersionMatch: opts.ResourceVersionMatch,
		ResourceVersion:      opts.ResourceVersion,
	}
	if !rs.isCRD {
		return rs.restClient.Get().Resource(rs.resource).VersionedParams(listOpts, scheme.ParameterCodec).Do(ctx).Into(listObj)
	} else {

		result := rs.restClient.Get().Resource(rs.resource).SpecificallyVersionedParams(listOpts, dynamicParameterCodec, versionV1).Do(ctx)
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
}

func (rs *apiServerStorage) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	timeoutSeconds := int64(minWatchTimeout.Seconds() * (rand.Float64() + 1.0))

	listOpts := &metav1.ListOptions{
		ResourceVersion:     opts.ResourceVersion,
		Watch:               true,
		TimeoutSeconds:      &timeoutSeconds,
		AllowWatchBookmarks: true,
	}
	if !rs.isCRD {
		return rs.restClient.Get().Resource(rs.resource).VersionedParams(listOpts, scheme.ParameterCodec).Watch(ctx)
	} else {
		return rs.restClient.Get().Resource(rs.resource).SpecificallyVersionedParams(listOpts, dynamicParameterCodec, versionV1).Watch(ctx)
	}
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

func (rs *apiServerStorage) ReadinessCheck() error {
	return ErrNoSupport
}

func (rs *apiServerStorage) RequestWatchProgress(ctx context.Context) error {
	return ErrNoSupport
}
