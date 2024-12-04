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
	"testing"

	"github.com/stretchr/testify/assert"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	ctesting "github.com/openyurtio/openyurt/pkg/yurthub/proxy/multiplexer/testing"
)

func TestFilterWatch_ResultChan(t *testing.T) {
	t.Run("test filter endpointslices", func(t *testing.T) {
		source := watch.NewFake()
		filter := &ctesting.IgnoreEndpointslicesWithNodeName{IgnoreNodeName: "node1"}
		fw := newFilterWatch(source, filter)

		go func() {
			source.Add(mockEndpointslices())
		}()

		assertFilterWatchEvent(t, fw)
	})

	t.Run("test cacheable object", func(t *testing.T) {
		source := watch.NewFake()
		filter := &ctesting.IgnoreEndpointslicesWithNodeName{IgnoreNodeName: "node1"}

		fw := newFilterWatch(source, filter)

		go func() {
			source.Add(mockCacheableObject())
		}()

		assertFilterWatchEvent(t, fw)
	})
}

func mockEndpointslices() *discoveryv1.EndpointSlice {
	node1 := "node1"
	node2 := "node2"

	return &discoveryv1.EndpointSlice{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EndpointSlice",
			APIVersion: "discoveryv1discoveryv1.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns-12345",
			Namespace: "kube-system",
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"172.16.0.1"},
				NodeName:  &node1,
			},
			{
				Addresses: []string{"172.17.0.1"},
				NodeName:  &node2,
			},
		},
	}
}

func assertFilterWatchEvent(t testing.TB, fw watch.Interface) {
	t.Helper()

	event := <-fw.ResultChan()
	endpointslice, ok := event.Object.(*discoveryv1.EndpointSlice)

	assert.Equal(t, true, ok)
	assert.Equal(t, 1, len(endpointslice.Endpoints))
	assert.Equal(t, *endpointslice.Endpoints[0].NodeName, "node2")
}

func mockCacheableObject() *ctesting.MockCacheableObject {
	return &ctesting.MockCacheableObject{
		Obj: mockEndpointslices(),
	}
}

func TestFilterWatch_Stop(t *testing.T) {
	source := watch.NewFake()
	filter := &ctesting.IgnoreEndpointslicesWithNodeName{IgnoreNodeName: "node1"}
	fw := newFilterWatch(source, filter)

	fw.Stop()

	assert.Equal(t, true, source.IsStopped())
}
