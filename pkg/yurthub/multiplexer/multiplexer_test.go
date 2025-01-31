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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	kstorage "k8s.io/apiserver/pkg/storage"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

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

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	}

	scm := NewRequestMultiplexerManager(dsm, restMapperManager, poolScopeResources)
	cache, _, _ := scm.ResourceCache(serviceGVR)
	wait.PollUntilContextCancel(context.Background(), 100*time.Millisecond, true, func(context.Context) (done bool, err error) {
		if cache.ReadinessCheck() == nil {
			return true, nil
		}
		return false, nil
	})

	serviceList := &v1.ServiceList{}
	err = cache.GetList(context.Background(), "", mockListOptions(), serviceList)

	assert.Nil(t, err)
	assert.Equal(t, []v1.Service{
		*newService(metav1.NamespaceDefault, "nginx"),
		*newService(metav1.NamespaceSystem, "coredns"),
	}, serviceList.Items)
}
