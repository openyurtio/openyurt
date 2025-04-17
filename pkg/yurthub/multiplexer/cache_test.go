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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"

	ystorage "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

var serviceGVR = &schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "services",
}

var newServiceFunc = func() runtime.Object {
	return &v1.Service{}
}

var newServiceListFunc = func() runtime.Object {
	return &v1.ServiceList{}
}

var newFooFunc = func() runtime.Object {
	return &unstructured.Unstructured{}
}

var newFooListFunc = func() runtime.Object {
	return &unstructured.UnstructuredList{}
}

func TestResourceCache_GetList(t *testing.T) {
	storage := ystorage.NewFakeServiceStorage(
		[]v1.Service{
			*newService(metav1.NamespaceSystem, "coredns"),
			*newService(metav1.NamespaceDefault, "nginx"),
		})

	cache, _, _ := NewResourceCache(
		storage,
		serviceGVR,
		&ResourceCacheConfig{
			KeyFunc,
			newServiceFunc,
			newServiceListFunc,
			AttrsFunc,
		}, false,
	)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	for k, tc := range map[string]struct {
		key                 string
		expectedServiceList *v1.ServiceList
	}{
		"all namespace": {
			"",
			&v1.ServiceList{
				ListMeta: metav1.ListMeta{
					ResourceVersion: "100",
				},
				Items: []v1.Service{
					*newService(metav1.NamespaceDefault, "nginx"),
					*newService(metav1.NamespaceSystem, "coredns"),
				},
			},
		},
		"default namespace": {
			"/default",
			&v1.ServiceList{
				ListMeta: metav1.ListMeta{
					ResourceVersion: "100",
				},
				Items: []v1.Service{
					*newService(metav1.NamespaceDefault, "nginx"),
				},
			},
		},
	} {
		t.Run(k, func(t *testing.T) {
			serviceList := &v1.ServiceList{}
			err := cache.GetList(context.Background(), tc.key, mockListOptions(), serviceList)

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedServiceList.Items, serviceList.Items)
		})
	}
}
func TestCRDResourceCache_GetList(t *testing.T) {
	fooStorage := ystorage.NewFakeFooStorage(
		[]unstructured.Unstructured{
			*newFoo("default", "foo-1", "v1alpha1"),
			*newFoo("kube-system", "foo-2", "v1alpha1"),
		})

	cache, _, _ := NewResourceCache(
		fooStorage,
		&fooGVR,
		&ResourceCacheConfig{
			KeyFunc,
			newFooFunc,
			newFooListFunc,
			AttrsFunc,
		}, true,
	)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	for k, tc := range map[string]struct {
		key                 string
		expectedServiceList *unstructured.UnstructuredList
	}{
		"all namespace": {
			"",
			&unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					*newFoo("default", "foo-1", "v1alpha1"),
					*newFoo("kube-system", "foo-2", "v1alpha1"),
				},
			},
		},
		"default namespace": {
			"/default",
			&unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{
					*newFoo("default", "foo-1", "v1alpha1"),
				},
			},
		},
	} {
		t.Run(k, func(t *testing.T) {
			fooList := &unstructured.UnstructuredList{}
			err := cache.GetList(context.Background(), tc.key, mockListOptions(), fooList)

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedServiceList.Items, fooList.Items)
		})
	}
}

func mockListOptions() storage.ListOptions {
	return storage.ListOptions{
		ResourceVersion: "100",
		Recursive:       true,
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
	}
}
func TestCRDResourceCache_Watch(t *testing.T) {
	fooStorage := ystorage.NewFakeFooStorage(
		[]unstructured.Unstructured{
			*newFoo("kube-system", "foo", "v1alpha1"),
		})

	cache, _, err := NewResourceCache(
		fooStorage,
		&fooGVR,
		&ResourceCacheConfig{
			KeyFunc,
			newFooFunc,
			newFooListFunc,
			AttrsFunc,
		}, true,
	)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	assert.Nil(t, err)
	assertCRDCacheWatch(t, cache, fooStorage)
}
func TestResourceCache_Watch(t *testing.T) {
	fakeStorage := ystorage.NewFakeServiceStorage([]v1.Service{*newService(metav1.NamespaceSystem, "coredns")})

	cache, _, err := NewResourceCache(
		fakeStorage,
		serviceGVR,
		&ResourceCacheConfig{
			KeyFunc,
			newServiceFunc,
			newServiceListFunc,
			AttrsFunc,
		}, false,
	)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	assert.Nil(t, err)
	assertCacheWatch(t, cache, fakeStorage)
}

func mockWatchOptions() storage.ListOptions {
	var sendInitialEvents = true

	return storage.ListOptions{
		ResourceVersion: "100",
		Predicate: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.Everything(),
		},
		Recursive:         true,
		SendInitialEvents: &sendInitialEvents,
	}
}

func assertCacheWatch(t testing.TB, cache Interface, fs *ystorage.FakeServiceStorage) {
	receive, err := cache.Watch(context.TODO(), "/kube-system", mockWatchOptions())

	go func() {
		fs.AddWatchObject(newService(metav1.NamespaceSystem, "coredns2"))
	}()

	assert.Nil(t, err)
	event := <-receive.ResultChan()
	assert.Equal(t, watch.Added, event.Type)
}
func assertCRDCacheWatch(t testing.TB, cache Interface, fs *ystorage.FakeFooStorage) {
	receive, err := cache.Watch(context.TODO(), "/kube-system", mockWatchOptions())

	go func() {
		fs.AddWatchObject(newFoo("kube-system", "foo-2", "v1alpha1"))
	}()

	assert.Nil(t, err)
	event := <-receive.ResultChan()
	assert.Equal(t, watch.Added, event.Type)
}
