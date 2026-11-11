/*
Copyright 2026 The OpenYurt Authors.

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

package cachemanager

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// A cached list is written with ReplaceComponentList, which deletes every previously
// cached object that is absent from the response. That is only a safe thing to do when
// the response provably carries the COMPLETE set of matching objects.
//
// Two kinds of response do not, and both used to reach ReplaceComponentList:
//
//  1. a response to a list restricted to a single object by
//     ?fieldSelector=metadata.name=<name> — evidence about one object, never about the
//     collection;
//  2. one page of a paginated list (?limit=, or a continuation), which by construction
//     omits the objects on the other pages.
//
// Both are reachable by any client that can reach a yurthub proxy port, because
// cacheability is decided from the caller-supplied User-Agent (CanCacheFor) and the
// recorded per-key selector identity covers only label and field selectors — not limit
// and not continue. The damage is invisible until the node reboots while disconnected,
// at which point kubelet is served a near-empty pod list.
//
// These tests seed a component's cache with a complete list and then assert that neither
// kind of response is allowed to delete what it does not contain.
func newTruncationTestCacheManager(t *testing.T, dir string) (CacheManager, *serializer.SerializerManager) {
	t.Helper()
	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("could not create disk storage, %v", err)
	}
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(dir)
	if err != nil {
		t.Fatalf("could not create RESTMapper manager, %v", err)
	}
	serializerM := serializer.NewSerializerManager()
	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	return NewCacheManager(NewStorageWrapper(dStorage), serializerM, restRESTMapperMgr, configManager), serializerM
}

// cacheListResponse drives one list response through the same middleware chain the proxy
// builds, so User-Agent, RequestInfo and content type are populated exactly as in production.
func cacheListResponse(t *testing.T, cm CacheManager, serializerM *serializer.SerializerManager, path string, obj runtime.Object) error {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("User-Agent", "kubelet")
	req.Header.Set("Accept", "application/json")
	req.RemoteAddr = "127.0.0.1"

	var cacheErr error
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		info, _ := request.RequestInfoFrom(ctx)
		reqContentType, _ := util.ReqContentTypeFrom(ctx)
		ctx = util.WithRespContentType(ctx, reqContentType)
		req = req.WithContext(ctx)

		gvr := schema.GroupVersionResource{Group: info.APIGroup, Version: info.APIVersion, Resource: info.Resource}
		s := serializerM.CreateSerializer(reqContentType, gvr.Group, gvr.Version, gvr.Resource)
		encoder, err := s.Encoder(reqContentType, nil)
		if err != nil {
			t.Fatalf("could not create encoder, %v", err)
		}
		buf := bytes.NewBuffer([]byte{})
		if err := encoder.Encode(obj, buf); err != nil {
			t.Fatalf("could not encode input object, %v", err)
		}
		cacheErr = cm.CacheResponse(req, io.NopCloser(buf), nil)
	})

	handler = proxyutil.WithRequestContentType(handler)
	handler = proxyutil.WithListRequestSelector(handler)
	handler = proxyutil.WithRequestClientComponent(handler)
	handler = proxyutil.WithPartialObjectMetadataRequest(handler)
	handler = filters.WithRequestInfo(handler, newTestRequestInfoResolver())
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return cacheErr
}

func pod(name, rv string) v1.Pod {
	return v1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", ResourceVersion: rv},
		Spec:       v1.PodSpec{NodeName: "mynode"},
	}
}

func completePodList(rv string, pods ...v1.Pod) *v1.PodList {
	return &v1.PodList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"},
		ListMeta: metav1.ListMeta{ResourceVersion: rv},
		Items:    pods,
	}
}

// cachedPodNames returns the pod names currently cached for the kubelet component.
func cachedPodNames(t *testing.T, cm CacheManager) []string {
	t.Helper()
	req, _ := http.NewRequest("GET", "/api/v1/pods?fieldSelector=spec.nodeName=mynode", nil)
	req.Header.Set("User-Agent", "kubelet")
	req.RemoteAddr = "127.0.0.1"

	var names []string
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		obj, err := cm.QueryCache(req)
		if err != nil {
			t.Logf("QueryCache returned error: %v", err)
			return
		}
		list, ok := obj.(*v1.PodList)
		if !ok {
			t.Fatalf("expected a *v1.PodList, got %T", obj)
		}
		for i := range list.Items {
			names = append(names, list.Items[i].Name)
		}
	})
	handler = proxyutil.WithRequestContentType(handler)
	handler = proxyutil.WithListRequestSelector(handler)
	handler = proxyutil.WithRequestClientComponent(handler)
	handler = filters.WithRequestInfo(handler, newTestRequestInfoResolver())
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return names
}

// A list narrowed to a single object with ?fieldSelector=metadata.name= must never be
// treated as evidence about the whole collection. An empty response to such a request
// (the named object does not exist) used to delete every pod cached for the component.
func TestNameSelectorListDoesNotWipeCollection(t *testing.T) {
	dir := t.TempDir()
	cm, serializerM := newTruncationTestCacheManager(t, dir)

	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
		completePodList("10", pod("pod-a", "1"), pod("pod-b", "2"), pod("pod-c", "3"))); err != nil {
		t.Fatalf("seeding the cache failed: %v", err)
	}
	if got := cachedPodNames(t, cm); len(got) != 3 {
		t.Fatalf("precondition failed: expected 3 cached pods, got %d (%v)", len(got), got)
	}

	// The named pod does not exist, so the apiserver answers with an empty list.
	// It is evidence that "pod-zzz" is absent, and nothing more.
	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=metadata.name=pod-zzz",
		completePodList("11")); err != nil {
		t.Fatalf("caching the name-selector response failed: %v", err)
	}

	got := cachedPodNames(t, cm)
	if len(got) != 3 {
		t.Errorf("an empty response to a metadata.name list deleted the cached collection: expected 3 pods still cached, got %d (%v)", len(got), got)
	}
}

// RequestInfo.Name is NOT a reliable way to recognise a metadata.name list.
// requestinfo.go assigns it only when the value is a valid path segment, and
// path.IsValidPathSegmentName rejects "." and ".." outright and anything containing "/" or
// "%". For those values the request is still a name-selector list — the apiserver answers
// it with an empty list — but info.Name is empty, so a guard keyed on info.Name lets the
// response through to ReplaceComponentList and the whole collection is deleted.
//
// The read path already gets this right (queryListObject uses
// isListRequestWithNameFieldSelector), so the write path has to agree with it.
func TestOddNameSelectorValuesDoNotWipeCollection(t *testing.T) {
	for _, name := range []string{"..", ".", "", "a%2Fb", "a%25b", "a/b"} {
		t.Run("metadata.name="+name, func(t *testing.T) {
			dir := t.TempDir()
			cm, serializerM := newTruncationTestCacheManager(t, dir)

			if err := cacheListResponse(t, cm, serializerM,
				"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
				completePodList("10", pod("pod-a", "1"), pod("pod-b", "2"), pod("pod-c", "3"))); err != nil {
				t.Fatalf("seeding the cache failed: %v", err)
			}
			if got := cachedPodNames(t, cm); len(got) != 3 {
				t.Fatalf("precondition failed: expected 3 cached pods, got %d (%v)", len(got), got)
			}

			// The apiserver answers with an empty list: no object has this name.
			if err := cacheListResponse(t, cm, serializerM,
				"/api/v1/pods?fieldSelector="+url.QueryEscape("metadata.name="+name),
				completePodList("11")); err != nil {
				t.Fatalf("caching the name-selector response failed: %v", err)
			}

			if got := cachedPodNames(t, cm); len(got) != 3 {
				t.Errorf("an empty response to metadata.name=%q deleted the cached collection: expected 3 pods still cached, got %d (%v)", name, len(got), got)
			}
		})
	}
}

// One page of a paginated list omits the objects on every other page, so it must never
// be allowed to delete them. Both ends of the sequence matter: the first page is
// identified by a continue token in the RESPONSE, and the last page — which carries an
// empty continue token and is otherwise indistinguishable from a complete list — only by
// the continue parameter on the REQUEST.
func TestPaginatedListDoesNotWipeCollection(t *testing.T) {
	seed := func(t *testing.T, cm CacheManager, serializerM *serializer.SerializerManager) {
		if err := cacheListResponse(t, cm, serializerM,
			"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
			completePodList("10", pod("pod-a", "1"), pod("pod-b", "2"), pod("pod-c", "3"))); err != nil {
			t.Fatalf("seeding the cache failed: %v", err)
		}
		if got := cachedPodNames(t, cm); len(got) != 3 {
			t.Fatalf("precondition failed: expected 3 cached pods, got %d (%v)", len(got), got)
		}
	}

	t.Run("first page, continue token in the response", func(t *testing.T) {
		dir := t.TempDir()
		cm, serializerM := newTruncationTestCacheManager(t, dir)
		seed(t, cm, serializerM)

		firstPage := completePodList("11", pod("pod-a", "4"))
		firstPage.ListMeta.Continue = "some-continue-token"
		if err := cacheListResponse(t, cm, serializerM,
			"/api/v1/pods?fieldSelector=spec.nodeName=mynode&limit=1", firstPage); err != nil {
			t.Fatalf("caching the first page failed: %v", err)
		}

		if got := cachedPodNames(t, cm); len(got) != 3 {
			t.Errorf("the first page of a paginated list deleted the pods on the other pages: expected 3 pods still cached, got %d (%v)", len(got), got)
		}
	})

	t.Run("last page, continue token only on the request", func(t *testing.T) {
		dir := t.TempDir()
		cm, serializerM := newTruncationTestCacheManager(t, dir)
		seed(t, cm, serializerM)

		// The final page of a paginated sequence: its response continue token is empty,
		// exactly like a complete list. Only the request tells them apart.
		lastPage := completePodList("11", pod("pod-c", "5"))
		if err := cacheListResponse(t, cm, serializerM,
			"/api/v1/pods?fieldSelector=spec.nodeName=mynode&limit=1&continue=some-continue-token", lastPage); err != nil {
			t.Fatalf("caching the last page failed: %v", err)
		}

		if got := cachedPodNames(t, cm); len(got) != 3 {
			t.Errorf("the last page of a paginated list deleted the pods on the earlier pages: expected 3 pods still cached, got %d (%v)", len(got), got)
		}
	})
}

// Not creating the collection directory for an empty incomplete response is a deliberate
// simplification, and it rests on queryListObject answering a metadata.name list with an
// empty list rather than an error when nothing is cached. Assert that, so the
// simplification cannot silently become an offline "not found".
func TestNameSelectorListOnColdCacheReturnsEmptyList(t *testing.T) {
	dir := t.TempDir()
	cm, serializerM := newTruncationTestCacheManager(t, dir)

	// Nothing has ever been cached for this component.
	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=metadata.name=pod-zzz",
		completePodList("11")); err != nil {
		t.Fatalf("caching an empty name-selector response on a cold cache failed: %v", err)
	}

	req, _ := http.NewRequest("GET", "/api/v1/pods?fieldSelector=metadata.name=pod-zzz", nil)
	req.Header.Set("User-Agent", "kubelet")
	req.RemoteAddr = "127.0.0.1"
	var queryErr error
	var got runtime.Object
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got, queryErr = cm.QueryCache(req)
	})
	handler = proxyutil.WithRequestContentType(handler)
	handler = proxyutil.WithListRequestSelector(handler)
	handler = proxyutil.WithRequestClientComponent(handler)
	handler = filters.WithRequestInfo(handler, newTestRequestInfoResolver())
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if queryErr != nil {
		t.Fatalf("an offline metadata.name list on a cold cache must return an empty list, got error: %v", queryErr)
	}
	list, ok := got.(*v1.PodList)
	if !ok {
		t.Fatalf("expected a *v1.PodList, got %T", got)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected an empty list, got %d items", len(list.Items))
	}
}

// A list pinned to one exact resourceVersion describes the collection as it was then, so
// it must not delete objects created since.
func TestPointInTimeListDoesNotWipeCollection(t *testing.T) {
	dir := t.TempDir()
	cm, serializerM := newTruncationTestCacheManager(t, dir)

	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
		completePodList("100", pod("pod-a", "100"), pod("pod-b", "101"), pod("pod-c", "102"))); err != nil {
		t.Fatalf("seeding the cache failed: %v", err)
	}

	// A read pinned to an older version: pod-b and pod-c did not exist yet at rv 50.
	older := completePodList("50", pod("pod-a", "40"))
	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode&resourceVersion=50&resourceVersionMatch=Exact",
		older); err != nil {
		t.Fatalf("caching the point-in-time response failed: %v", err)
	}

	if got := cachedPodNames(t, cm); len(got) != 3 {
		t.Errorf("a point-in-time list rolled the cache back: expected 3 pods still cached, got %d (%v)", len(got), got)
	}
}

// The fix has two halves — do not delete, and still cache what the response carries. The
// second half was initially unasserted, so a no-op would have passed. Assert that an
// object appearing only on a paginated page really is written.
func TestIncompleteListStillCachesItsOwnObjects(t *testing.T) {
	dir := t.TempDir()
	cm, serializerM := newTruncationTestCacheManager(t, dir)

	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
		completePodList("10", pod("pod-a", "1"))); err != nil {
		t.Fatalf("seeding the cache failed: %v", err)
	}

	// pod-d is new and arrives only on a page of a paginated list.
	page := completePodList("11", pod("pod-d", "12"))
	page.ListMeta.Continue = "more"
	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode&limit=1", page); err != nil {
		t.Fatalf("caching the page failed: %v", err)
	}

	got := cachedPodNames(t, cm)
	found := false
	for _, n := range got {
		if n == "pod-d" {
			found = true
		}
	}
	if !found {
		t.Errorf("an object carried by a paginated page must still be cached: got %v, expected it to contain pod-d", got)
	}
	if len(got) != 2 {
		t.Errorf("expected pod-a to survive alongside pod-d, got %v", got)
	}
}

// A genuinely complete list must keep pruning, because that is how an object deleted in
// the cloud leaves the node's cache. This is the behaviour the two fixes above must not
// disturb, and it is asserted here so a future change cannot quietly trade it away.
func TestCompleteListStillPrunes(t *testing.T) {
	dir := t.TempDir()
	cm, serializerM := newTruncationTestCacheManager(t, dir)

	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
		completePodList("10", pod("pod-a", "1"), pod("pod-b", "2"), pod("pod-c", "3"))); err != nil {
		t.Fatalf("seeding the cache failed: %v", err)
	}

	// pod-b and pod-c were deleted in the cloud; a complete relist must evict them.
	if err := cacheListResponse(t, cm, serializerM,
		"/api/v1/pods?fieldSelector=spec.nodeName=mynode",
		completePodList("12", pod("pod-a", "6"))); err != nil {
		t.Fatalf("caching the complete relist failed: %v", err)
	}

	got := cachedPodNames(t, cm)
	if len(got) != 1 || got[0] != "pod-a" {
		t.Errorf("a complete list must still prune objects deleted in the cloud: expected only pod-a cached, got %v", got)
	}
}
