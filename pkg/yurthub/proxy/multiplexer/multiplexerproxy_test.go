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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	multiplexerstorage "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	ctesting "github.com/openyurtio/openyurt/pkg/yurthub/proxy/multiplexer/testing"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
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

func mockCacheMap() map[string]storage.Interface {
	return map[string]storage.Interface{
		endpointSliceGVR.String(): multiplexerstorage.NewFakeEndpointSliceStorage(
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

var (
	fooGVR = schema.GroupVersionResource{
		Group:    "samplecontroller.k8s.io",
		Version:  "v1alpha1",
		Resource: "foos",
	}
	fooGV = schema.GroupVersion{
		Group:   "samplecontroller.k8s.io",
		Version: "v1alpha1",
	}
	mockFoos = []unstructured.Unstructured{
		*newFoo("default", "foo-1", "v1alpha1"),
		*newFoo("kube-system", "foo-2", "v1alpha1"),
	}
)

func mockFooStorage() map[string]storage.Interface {
	return map[string]storage.Interface{
		fooGVR.String(): multiplexerstorage.NewFakeFooStorage(
			mockFoos,
		),
	}
}

func newFoo(namespace, name, version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": version,
			"kind":       "Foo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"exampleField": "test-value",
			},
		},
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
func TestShareProxy_CRD_ServeHTTP_LIST(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
		{Group: "samplecontroller.k8s.io", Version: "v1alpha1", Resource: "foos"},
	}

	for k, tc := range map[string]struct {
		filterFinder    filter.FilterFinder
		url             string
		expectedFooList *unstructured.UnstructuredList
		err             error
	}{
		"test list endpoint slices no filter": {
			filterFinder:    &ctesting.EmptyFilterManager{},
			url:             "/apis/samplecontroller.k8s.io/v1alpha1/foos",
			expectedFooList: expectFooListNoFilter(),
			err:             nil,
		},
		"test list endpoint slice with namespace": {
			filterFinder:    &ctesting.EmptyFilterManager{},
			url:             "/apis/samplecontroller.k8s.io/v1alpha1/namespaces/default/foos",
			expectedFooList: expectFooListWithNamespace(),
			err:             nil,
		},
	} {
		t.Run(k, func(t *testing.T) {
			w := &httptest.ResponseRecorder{
				Body: &bytes.Buffer{},
			}

			healthChecher := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
			loadBalancer := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, nil, healthChecher, nil, context.Background().Done())
			dsm := multiplexerstorage.NewDummyStorageManager(mockFooStorage())
			cfg := &config.YurtHubConfiguration{
				PoolScopeResources:       poolScopeResources,
				RESTMapperManager:        restMapperManager,
				SharedFactory:            factory,
				LoadBalancerForLeaderHub: loadBalancer,
			}
			rmm := multiplexer.NewRequestMultiplexerManager(cfg, dsm, healthChecher)

			restMapperManager.UpdateKind(schema.GroupVersionKind{
				Group:   "samplecontroller.k8s.io",
				Version: "v1alpha1",
				Kind:    "Foo",
			})
			informerSynced := func() bool {
				return rmm.Ready(&schema.GroupVersionResource{
					Group:    "samplecontroller.k8s.io",
					Version:  "v1alpha1",
					Resource: "foos",
				})
			}
			stopCh := make(chan struct{})
			if ok := cache.WaitForCacheSync(stopCh, informerSynced); !ok {
				t.Errorf("configuration manager is not ready")
				return
			}
			sp := NewMultiplexerProxy(tc.filterFinder,
				rmm,
				restMapperManager,
				make(<-chan struct{}))

			sp.ServeHTTP(w, newEndpointSliceListRequest(tc.url))

			result := DeepEqualUnstructured(tc.expectedFooList, decodeUnstructedSliceList(w.Body.Bytes()))
			assert.True(t, result, w.Body.String())
		})
	}
}
func DeepEqualUnstructured(a, b *unstructured.UnstructuredList) bool {
	if len(a.Items) != len(b.Items) {
		return false
	}
	return true
}
func TestShareProxy_ServeHTTP_LIST(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
		{Group: "samplecontroller.k8s.io", Version: "v1alpha1", Resource: "foos"},
	}

	for k, tc := range map[string]struct {
		filterFinder              filter.FilterFinder
		url                       string
		expectedEndPointSliceList *discovery.EndpointSliceList
		err                       error
	}{
		"test list endpoint slices no filter": {
			filterFinder:              &ctesting.EmptyFilterManager{},
			url:                       "/apis/discovery.k8s.io/v1/endpointslices",
			expectedEndPointSliceList: expectEndpointSliceListNoFilter(),
			err:                       nil,
		},
		"test list endpoint slice with filter": {
			filterFinder: &ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			url:                       "/apis/discovery.k8s.io/v1/endpointslices",
			expectedEndPointSliceList: expectEndpointSliceListWithFilter(),
			err:                       nil,
		},
		"test list endpoint slice with namespace": {
			filterFinder: &ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			url:                       "/apis/discovery.k8s.io/v1/namespaces/default/endpointslices",
			expectedEndPointSliceList: expectEndpointSliceListWithNamespace(),
			err:                       nil,
		},
	} {
		t.Run(k, func(t *testing.T) {
			w := &httptest.ResponseRecorder{
				Body: &bytes.Buffer{},
			}

			healthChecher := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
			loadBalancer := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, nil, healthChecher, nil, context.Background().Done())
			dsm := multiplexerstorage.NewDummyStorageManager(mockCacheMap())
			cfg := &config.YurtHubConfiguration{
				PoolScopeResources:       poolScopeResources,
				RESTMapperManager:        restMapperManager,
				SharedFactory:            factory,
				LoadBalancerForLeaderHub: loadBalancer,
			}
			rmm := multiplexer.NewRequestMultiplexerManager(cfg, dsm, healthChecher)

			informerSynced := func() bool {
				return rmm.Ready(&schema.GroupVersionResource{
					Group:    "discovery.k8s.io",
					Version:  "v1",
					Resource: "endpointslices",
				})
			}
			stopCh := make(chan struct{})
			if ok := cache.WaitForCacheSync(stopCh, informerSynced); !ok {
				t.Errorf("configuration manager is not ready")
				return
			}

			sp := NewMultiplexerProxy(tc.filterFinder,
				rmm,
				restMapperManager,
				make(<-chan struct{}))

			sp.ServeHTTP(w, newEndpointSliceListRequest(tc.url))

			result := equalEndpointSliceLists(tc.expectedEndPointSliceList, decodeEndpointSliceList(w.Body.Bytes()))
			assert.True(t, result, w.Body.String())
		})
	}
}

func expectFooListNoFilter() *unstructured.UnstructuredList {
	unstructuredList := &unstructured.UnstructuredList{}
	unstructuredList.Items = []unstructured.Unstructured{
		*newFoo("default", "foo-1", "v1alpha1"),
		*newFoo("kube-system", "foo-2", "v1alpha1"),
	}
	unstructuredList.SetResourceVersion("100")
	return unstructuredList
}
func expectFooListWithNamespace() *unstructured.UnstructuredList {
	unstructuredList := &unstructured.UnstructuredList{}
	unstructuredList.Items = []unstructured.Unstructured{
		*newFoo("default", "foo-1", "v1alpha1"),
	}
	unstructuredList.SetResourceVersion("100")
	return unstructuredList
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

func decodeEndpointSliceList(b []byte) *discovery.EndpointSliceList {
	discoveryv1Codec := scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(discoveryGV), scheme.Codecs.UniversalDecoder(discoveryGV), discoveryGV, discoveryGV)
	epsList := &discovery.EndpointSliceList{}
	err := runtime.DecodeInto(discoveryv1Codec, b, epsList)
	if err != nil {
		return nil
	}
	return epsList
}

func decodeUnstructedSliceList(b []byte) *unstructured.UnstructuredList {

	epsList := &unstructured.UnstructuredList{}
	codec := unstructured.UnstructuredJSONScheme
	err := runtime.DecodeInto(codec, b, epsList)
	if err != nil {
		return nil
	}
	return epsList
}

func TestShareProxy_ServeHTTP_WATCH(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	poolScopeResources := []schema.GroupVersionResource{
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
	}

	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	for k, tc := range map[string]struct {
		filterFinder       filter.FilterFinder
		url                string
		expectedWatchEvent *metav1.WatchEvent
		err                error
	}{
		"test watch endpointslice no filter": {
			filterFinder:       &ctesting.EmptyFilterManager{},
			url:                "/apis/discovery.k8s.io/v1/endpointslices?watch=true&&resourceVersion=100&&timeoutSeconds=3",
			expectedWatchEvent: expectedWatchEventNoFilter(),
			err:                nil,
		},
		"test watch endpointslice with filter": {
			filterFinder: &ctesting.FakeEndpointSliceFilter{
				NodeName: "node1",
			},
			url:                "/apis/discovery.k8s.io/v1/endpointslices?watch=true&&resourceVersion=100&&timeoutSeconds=3",
			expectedWatchEvent: expectedWatchEventWithFilter(),
			err:                nil,
		},
	} {
		t.Run(k, func(t *testing.T) {
			healthChecher := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
			loadBalancer := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, nil, healthChecher, nil, context.Background().Done())

			dsm := multiplexerstorage.NewDummyStorageManager(mockCacheMap())
			cfg := &config.YurtHubConfiguration{
				PoolScopeResources:       poolScopeResources,
				RESTMapperManager:        restMapperManager,
				SharedFactory:            factory,
				LoadBalancerForLeaderHub: loadBalancer,
			}
			rmm := multiplexer.NewRequestMultiplexerManager(cfg, dsm, healthChecher)

			informerSynced := func() bool {
				return rmm.Ready(&schema.GroupVersionResource{
					Group:    "discovery.k8s.io",
					Version:  "v1",
					Resource: "endpointslices",
				})
			}
			stopCh := make(chan struct{})
			if ok := cache.WaitForCacheSync(stopCh, informerSynced); !ok {
				t.Errorf("configuration manager is not ready")
				return
			}

			sp := NewMultiplexerProxy(
				tc.filterFinder,
				rmm,
				restMapperManager,
				make(<-chan struct{}),
			)

			req := newWatchEndpointSliceRequest(tc.url)
			w := newWatchResponse()

			go func() {
				sp.ServeHTTP(w, req)
			}()
			generateWatchEvent(dsm)

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

func generateWatchEvent(sp multiplexerstorage.StorageProvider) {
	fs, err := sp.ResourceStorage(&endpointSliceGVR, false)
	if err != nil {
		return
	}

	fess, ok := fs.(*multiplexerstorage.FakeEndpointSliceStorage)
	if ok {
		fess.AddWatchObject(newEndpointSlice(metav1.NamespaceSystem, "coredns-23456", "102", mockEndpoints))
	}
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

func equalEndpointSlice(a, b discovery.EndpointSlice) bool {
	if len(a.Endpoints) != len(b.Endpoints) {
		return false
	}

	countA := make(map[string]int)
	for _, endpoint := range a.Endpoints {
		key := endpointKey(endpoint)
		countA[key]++
	}

	for _, endpoint := range b.Endpoints {
		key := endpointKey(endpoint)
		if countA[key] == 0 {
			return false
		}

		countA[key]--
		if countA[key] == 0 {
			delete(countA, key)
		}
	}

	return len(countA) == 0
}

func endpointKey(endpoint discovery.Endpoint) string {
	return fmt.Sprintf("%v/%s", endpoint.Addresses, *endpoint.NodeName)
}

func equalEndpointSliceLists(a, b *discovery.EndpointSliceList) bool {
	if len(a.Items) != len(b.Items) {
		return false
	}

	sort.Slice(a.Items, func(i, j int) bool {
		return endpointSliceKey(a.Items[i]) < endpointSliceKey(a.Items[j])
	})
	sort.Slice(b.Items, func(i, j int) bool {
		return endpointSliceKey(b.Items[i]) < endpointSliceKey(b.Items[j])
	})

	for i := range a.Items {
		if !equalEndpointSlice(a.Items[i], b.Items[i]) {
			return false
		}
	}
	return true
}

func endpointSliceKey(slice discovery.EndpointSlice) string {
	keys := make([]string, len(slice.Endpoints))
	for i, endpoint := range slice.Endpoints {
		keys[i] = endpointKey(endpoint)
	}
	sort.Strings(keys)
	return fmt.Sprint(keys)
}
