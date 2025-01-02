/*
Copyright 2024 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	ctesting "github.com/openyurtio/openyurt/pkg/yurthub/proxy/multiplexer/testing"
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

func mockCacheMap() map[string]multiplexer.Interface {
	return map[string]multiplexer.Interface{
		endpointSliceGVR.String(): storage.NewFakeEndpointSliceStorage(
			[]discovery.EndpointSlice{
				*newEndpointSlice(metav1.NamespaceSystem, "coredns-12345", "", mockEndpoints),
				*newEndpointSlice(metav1.NamespaceDefault, "nginx", "", mockEndpoints),
			},
		),
	}
}

func mockResourceCacheMap() map[string]*multiplexer.ResourceCacheConfig {
	return map[string]*multiplexer.ResourceCacheConfig{
		endpointSliceGVR.String(): {
			KeyFunc: multiplexer.KeyFunc,
			NewListFunc: func() runtime.Object {
				return &discovery.EndpointSliceList{}
			},
			NewFunc: func() runtime.Object {
				return &discovery.EndpointSlice{}
			},
			GetAttrsFunc: multiplexer.AttrsFunc,
		},
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

type wrapResponse struct {
	Done chan struct{}
	*httptest.ResponseRecorder
}

func (wr *wrapResponse) Write(buf []byte) (int, error) {
	l, err := wr.ResponseRecorder.Write(buf)
	wr.Done <- struct{}{}
	return l, err
}

func TestShareProxy_ServeHTTP_LIST(t *testing.T) {
	for _, tc := range []struct {
		tName                     string
		filterManager             filter.FilterFinder
		url                       string
		expectedEndPointSliceList *discovery.EndpointSliceList
		err                       error
	}{
		{
			"test list endpoint slices no filter",
			&ctesting.EmptyFilterManager{},
			"/apis/discovery.k8s.io/v1/endpointslices",
			expectEndpointSliceListNoFilter(),

			nil,
		},
		{
			"test list endpoint slice with filter",
			&ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			"/apis/discovery.k8s.io/v1/endpointslices",
			expectEndpointSliceListWithFilter(),
			nil,
		},
		{
			"test list endpoint slice with namespace",
			&ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			"/apis/discovery.k8s.io/v1/namespaces/default/endpointslices",
			expectEndpointSliceListWithNamespace(),
			nil,
		},
	} {
		t.Run(tc.tName, func(t *testing.T) {
			w := &httptest.ResponseRecorder{
				Body: &bytes.Buffer{},
			}

			sp, err := NewMultiplexerProxy(tc.filterManager,
				multiplexer.NewFakeCacheManager(mockCacheMap(), mockResourceCacheMap()),
				[]schema.GroupVersionResource{endpointSliceGVR},
				make(<-chan struct{}))

			assert.Equal(t, tc.err, err)

			sp.ServeHTTP(w, newEndpointSliceListRequest(tc.url))

			assert.Equal(t, string(encodeEndpointSliceList(tc.expectedEndPointSliceList)), w.Body.String())
		})
	}
}

func expectEndpointSliceListNoFilter() *discovery.EndpointSliceList {
	return &discovery.EndpointSliceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{
			ResourceVersion: "100",
		},
		Items: []discovery.EndpointSlice{
			*newEndpointSlice(metav1.NamespaceSystem, "coredns-12345", "", mockEndpoints),
			*newEndpointSlice(metav1.NamespaceDefault, "nginx", "", mockEndpoints),
		},
	}
}

func newStringPointer(str string) *string {
	return &str
}

func expectEndpointSliceListWithFilter() *discovery.EndpointSliceList {
	endpoints := []discovery.Endpoint{
		{
			Addresses: []string{"192.168.1.1"},
			NodeName:  newStringPointer("node2"),
		},
		{
			Addresses: []string{"192.168.2.3"},
			NodeName:  newStringPointer("node3"),
		},
	}

	return &discovery.EndpointSliceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{
			ResourceVersion: "100",
		},
		Items: []discovery.EndpointSlice{
			*newEndpointSlice(metav1.NamespaceSystem, "coredns-12345", "", endpoints),
			*newEndpointSlice(metav1.NamespaceDefault, "nginx", "", endpoints),
		},
	}
}

func expectEndpointSliceListWithNamespace() *discovery.EndpointSliceList {
	endpoints := []discovery.Endpoint{
		{
			Addresses: []string{"192.168.1.1"},
			NodeName:  newStringPointer("node2"),
		},
		{
			Addresses: []string{"192.168.2.3"},
			NodeName:  newStringPointer("node3"),
		},
	}

	return &discovery.EndpointSliceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{
			ResourceVersion: "100",
		},
		Items: []discovery.EndpointSlice{
			*newEndpointSlice(metav1.NamespaceDefault, "nginx", "", endpoints),
		},
	}
}

func newEndpointSliceListRequest(url string) *http.Request {
	req := httptest.NewRequest("GET", url, &bytes.Buffer{})

	ctx := req.Context()
	req = req.WithContext(request.WithRequestInfo(ctx, resolverRequestInfo(req)))

	return req
}

func resolverRequestInfo(req *http.Request) *request.RequestInfo {
	cfg := &server.Config{
		LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
	}
	resolver := server.NewRequestInfoResolver(cfg)
	info, _ := resolver.NewRequestInfo(req)
	return info
}

func encodeEndpointSliceList(endpointSliceList *discovery.EndpointSliceList) []byte {
	discoveryv1Codec := scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(discoveryGV), scheme.Codecs.UniversalDecoder(discoveryGV), discoveryGV, discoveryGV)

	str := runtime.EncodeOrDie(discoveryv1Codec, endpointSliceList)
	return []byte(str)
}

func TestShareProxy_ServeHTTP_WATCH(t *testing.T) {
	for _, tc := range []struct {
		tName              string
		filterManager      filter.FilterFinder
		url                string
		expectedWatchEvent *metav1.WatchEvent
		Err                error
	}{
		{"test watch endpointslice no filter",
			&ctesting.EmptyFilterManager{},
			"/apis/discovery.k8s.io/v1/endpointslices?watch=true&&resourceVersion=0&&timeoutSeconds=3",
			expectedWatchEventNoFilter(),
			nil,
		},
		{"test watch endpointslice with filter",
			&ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			"/apis/discovery.k8s.io/v1/endpointslices?watch=true&&resourceVersion=0&&timeoutSeconds=3",
			expectedWatchEventWithFilter(),
			nil,
		},
	} {
		t.Run(tc.tName, func(t *testing.T) {
			fcm := multiplexer.NewFakeCacheManager(mockCacheMap(), mockResourceCacheMap())

			sp, _ := NewMultiplexerProxy(
				tc.filterManager,
				fcm,
				[]schema.GroupVersionResource{endpointSliceGVR},
				make(<-chan struct{}),
			)

			req := newWatchEndpointSliceRequest(tc.url)
			w := newWatchResponse()

			go func() {
				sp.ServeHTTP(w, req)
			}()
			generateWatchEvent(fcm)

			assertWatchResp(t, tc.expectedWatchEvent, w)
		})
	}
}

func expectedWatchEventNoFilter() *metav1.WatchEvent {
	return &metav1.WatchEvent{
		Type: "ADDED",
		Object: runtime.RawExtension{
			Object: newEndpointSlice(metav1.NamespaceSystem, "coredns-23456", "101", mockEndpoints),
		},
	}
}

func expectedWatchEventWithFilter() *metav1.WatchEvent {
	endpoints := []discovery.Endpoint{
		{
			Addresses: []string{"192.168.1.1"},
			NodeName:  newStringPointer("node2"),
		},
		{
			Addresses: []string{"192.168.2.3"},
			NodeName:  newStringPointer("node3"),
		},
	}
	return &metav1.WatchEvent{
		Type: "ADDED",
		Object: runtime.RawExtension{
			Object: newEndpointSlice(metav1.NamespaceSystem, "coredns-23456", "101", endpoints),
		},
	}
}

func newWatchEndpointSliceRequest(url string) *http.Request {
	req := httptest.NewRequest("GET", url, &bytes.Buffer{})

	ctx := req.Context()
	req = req.WithContext(request.WithRequestInfo(ctx, resolverRequestInfo(req)))

	return req
}

func newWatchResponse() *wrapResponse {
	return &wrapResponse{
		make(chan struct{}),
		&httptest.ResponseRecorder{
			Body: &bytes.Buffer{},
		},
	}
}

func generateWatchEvent(fcm *multiplexer.FakeCacheManager) {
	fs, _, _ := fcm.ResourceCache(&endpointSliceGVR)

	fess, _ := fs.(*storage.FakeEndpointSliceStorage)
	fess.AddWatchObject(newEndpointSlice(metav1.NamespaceSystem, "coredns-23456", "102", mockEndpoints))
}

func assertWatchResp(t testing.TB, expectedWatchEvent *metav1.WatchEvent, w *wrapResponse) {
	t.Helper()

	select {
	case <-time.After(5 * time.Second):
		t.Errorf("wait watch timeout")
	case <-w.Done:
		assert.Equal(t, string(encodeWatchEventList(expectedWatchEvent)), w.Body.String())
	}
}

func encodeWatchEventList(watchEvent *metav1.WatchEvent) []byte {
	metav1Codec := scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(discoveryGV), scheme.Codecs.UniversalDecoder(discoveryGV), discoveryGV, discoveryGV)

	str := runtime.EncodeOrDie(metav1Codec, watchEvent)
	return []byte(str)
}
