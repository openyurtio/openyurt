/*
Copyright 2025 The OpenYurt Authors.

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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	discovery "k8s.io/api/discovery/v1"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	storage2 "k8s.io/apiserver/pkg/storage"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

var (
	discoveryGV = schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"}

	endpointSliceGVR = discoveryGV.WithResource("endpointslices")
)

var mockEndpoints = []discovery.Endpoint{
	{
		Addresses: []string{"192.168.0.1"},
		NodeName:  newStringPointer("node1"),
	},
	{
		Addresses: []string{"192.168.1.1"},
		NodeName:  newStringPointer("node2"),
	},
	{
		Addresses: []string{"192.168.2.3"},
		NodeName:  newStringPointer("node3"),
	},
}

func newStringPointer(str string) *string {
	return &str
}

func mockCacheMap() map[string]storage2.Interface {
	return map[string]storage2.Interface{
		endpointSliceGVR.String(): storage.NewFakeEndpointSliceStorage(
			[]discovery.EndpointSlice{
				*newEndpointSlice(metav1.NamespaceSystem, "coredns-12345", "", mockEndpoints),
				*newEndpointSlice(metav1.NamespaceDefault, "nginx", "", mockEndpoints),
			},
		),
	}
}

func newEndpointSlice(namespace string, name string, resourceVersion string, endpoints []discovery.Endpoint) *discovery.EndpointSlice {
	return &discovery.EndpointSlice{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EndpointSlice",
			APIVersion: "discovery.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: resourceVersion,
		},
		Endpoints: endpoints,
	}
}

func Test_FilterStoreManager_FilterStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	fsm := newFilterStoreManager(&config.YurtHubConfiguration{
		RESTMapperManager: restMapperManager,
	}, storage.NewDummyStorageManager(mockCacheMap()))

	store, err := fsm.FilterStore(&endpointSliceGVR)
	assert.Nil(t, err)

	wait.PollUntilContextTimeout(context.Background(), time.Second, 5*time.Second, true, func(ctx context.Context) (done bool, err error) {
		return store.ReadinessCheck() == nil, nil
	})

	objects, err := store.List(context.Background(), &metainternalversion.ListOptions{})
	assert.Nil(t, err)

	_, ok := objects.(*discovery.EndpointSliceList)
	assert.Equal(t, true, ok)

}
