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

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		},
	)

	for _, tc := range []struct {
		name                string
		key                 string
		expectedServiceList *v1.ServiceList
	}{
		{
			"all namespace",
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
		{
			"default namespace",
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
		serviceList := &v1.ServiceList{}
		err := cache.GetList(context.Background(), tc.key, mockListOptions(), serviceList)

		assert.Nil(t, err)
		assert.Equal(t, tc.expectedServiceList.Items, serviceList.Items)
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
		},
	)

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
