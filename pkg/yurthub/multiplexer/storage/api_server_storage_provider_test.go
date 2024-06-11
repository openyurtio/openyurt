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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/rest"
)

var serviceGVR = &schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "services",
}

var endpointSlicesGVR = &schema.GroupVersionResource{
	Group:    "discovery.k8s.io",
	Version:  "v1",
	Resource: "endpointslices",
}

func TestStorageManager_ResourceStorage(t *testing.T) {
	sm := NewStorageManager(&rest.Config{
		Host:      "http://127.0.0.1:10261",
		UserAgent: "share-hub",
	})

	for _, tc := range []struct {
		tName string
		gvr   *schema.GroupVersionResource
		Err   error
	}{
		{
			"get resource storage for services",
			serviceGVR,
			nil,
		},
		{
			"get resource storage for endpouintslices",
			endpointSlicesGVR,
			nil,
		},
	} {
		t.Run(tc.tName, func(t *testing.T) {
			restore, err := sm.ResourceStorage(tc.gvr)

			assert.Nil(t, err)
			assertResourceStore(t, tc.gvr, restore)
		})
	}
}

func assertResourceStore(t testing.TB, gvr *schema.GroupVersionResource, getRestStore storage.Interface) {
	t.Helper()

	store, ok := getRestStore.(*apiServerStorage)
	assert.Equal(t, true, ok)
	assert.Equal(t, gvr.Resource, store.resource)
	assert.Equal(t, gvr.GroupVersion(), store.restClient.APIVersion())
}
