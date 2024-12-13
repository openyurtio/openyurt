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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kstorage "k8s.io/apiserver/pkg/storage"

	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

func TestShareCacheManager_ResourceCacheConfig(t *testing.T) {
	svcStorage := storage.NewFakeServiceStorage([]v1.Service{*newService(metav1.NamespaceSystem, "coredns")})
	storageMap := map[string]kstorage.Interface{
		serviceGVR.String(): svcStorage,
	}

	sm := NewRequestsMultiplexerManager(
		storage.NewDummyStorageManager(storageMap))

	for _, tc := range []struct {
		tname               string
		gvr                 *schema.GroupVersionResource
		obj                 runtime.Object
		expectedKey         string
		expectedObjType     string
		expectedObjListType string
		expectedFieldSet    fields.Set
		namespaceScoped     bool
	}{
		{
			"generate resource config for services",
			&schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "services",
			},
			newService(metav1.NamespaceSystem, "coredns"),
			"/kube-system/coredns",
			"Service",
			"ServiceList",
			fields.Set{
				"metadata.name":      "coredns",
				"metadata.namespace": "kube-system",
			},
			true,
		},
		{
			"generate resource config for endpointslices",
			&schema.GroupVersionResource{
				Group:    "discovery.k8s.io",
				Version:  "v1",
				Resource: "endpointslices",
			},
			newEndpointSlice(),
			"/kube-system/coredns-12345",
			"EndpointSlice",
			"EndpointSliceList",
			fields.Set{
				"metadata.name":      "coredns-12345",
				"metadata.namespace": "kube-system",
			},
			true,
		},
		{
			"generate resource config for nodes",
			&schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "nodes",
			},
			newNode(),
			"/test",
			"Node",
			"NodeList",
			fields.Set{
				"metadata.name": "test",
			},
			false,
		},
	} {
		t.Run(tc.tname, func(t *testing.T) {
			rc, err := sm.ResourceCacheConfig(tc.gvr)

			assert.Nil(t, err)

			key, _ := rc.KeyFunc(tc.obj)
			assert.Equal(t, tc.expectedKey, key)

			obj := rc.NewFunc()
			assert.Equal(t, tc.expectedObjType, reflect.TypeOf(obj).Elem().Name())

			objList := rc.NewListFunc()
			assert.Equal(t, tc.expectedObjListType, reflect.TypeOf(objList).Elem().Name())

			_, fieldSet, _ := rc.GetAttrsFunc(tc.obj)
			assert.Equal(t, tc.expectedFieldSet, fieldSet)
		})
	}
}
func newService(namespace, name string) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func newEndpointSlice() *discovery.EndpointSlice {
	return &discovery.EndpointSlice{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EndpointSlice",
			APIVersion: "discovery.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "coredns-12345",
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{"192.168.0.10"},
			},
		},
	}
}

func newNode() *v1.Node {
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
}

func TestShareCacheManager_ResourceCache(t *testing.T) {
	svcStorage := storage.NewFakeServiceStorage(
		[]v1.Service{
			*newService(metav1.NamespaceSystem, "coredns"),
			*newService(metav1.NamespaceDefault, "nginx"),
		})

	storageMap := map[string]kstorage.Interface{
		serviceGVR.String(): svcStorage,
	}

	dsm := storage.NewDummyStorageManager(storageMap)
	scm := NewRequestsMultiplexerManager(dsm)
	cache, _, _ := scm.ResourceCache(serviceGVR)

	serviceList := &v1.ServiceList{}
	err := cache.GetList(context.Background(), "", mockListOptions(), serviceList)

	assert.Nil(t, err)
	assert.Equal(t, []v1.Service{
		*newService(metav1.NamespaceDefault, "nginx"),
		*newService(metav1.NamespaceSystem, "coredns"),
	}, serviceList.Items)
}
