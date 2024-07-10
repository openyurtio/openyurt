/*
Copyright 2020 The OpenYurt Authors.

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

package local

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	rootDir                   = "/tmp/cache-local"
	fakeClient                = fake.NewSimpleClientset()
	fakeSharedInformerFactory = informers.NewSharedInformerFactory(fakeClient, 0)
)

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestServeHTTPForWatch(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent string
		accept    string
		verb      string
		path      string
		code      int
		floor     time.Duration
		ceil      time.Duration
	}{
		"watch request": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes?watch=true&timeoutSeconds=5",
			code:      http.StatusOK,
			floor:     4 * time.Second,
			ceil:      6 * time.Second,
		},
		"watch request without timeout": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes?watch=true",
			code:      http.StatusOK,
			ceil:      61 * time.Second,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var start, end time.Time
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				start = time.Now()
				lp.ServeHTTP(w, req)
				end = time.Now()
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			if tt.floor.Seconds() != 0 {
				if start.Add(tt.floor).After(end) {
					t.Errorf("exec time is less than floor time %v", tt.floor)
				}
			}

			if start.Add(tt.ceil).Before(end) {
				t.Errorf("exec time is more than ceil time %v", tt.ceil)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestServeHTTPForWatchWithHealthyChange(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	cnt := 0
	fn := func() bool {
		cnt++
		return cnt > 2 // after 6 seconds, become healthy
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent string
		accept    string
		verb      string
		path      string
		code      int
		floor     time.Duration
		ceil      time.Duration
	}{
		"watch request": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes?watch=true&timeoutSeconds=10",
			code:      http.StatusOK,
			floor:     5 * time.Second,
			ceil:      7 * time.Second,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var start, end time.Time
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				start = time.Now()
				lp.ServeHTTP(w, req)
				end = time.Now()
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			if tt.floor.Seconds() != 0 {
				if start.Add(tt.floor).After(end) {
					t.Errorf("exec time is less than floor time %v", tt.floor)
				}
			}

			if start.Add(tt.ceil).Before(end) {
				t.Errorf("exec time is more than ceil time %v", tt.ceil)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}
func TestServeHTTPForWatchWithMinRequestTimeout(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 10*time.Second)

	testcases := map[string]struct {
		userAgent string
		accept    string
		verb      string
		path      string
		code      int
		floor     time.Duration
		ceil      time.Duration
	}{
		"watch request": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes?watch=true&timeoutSeconds=5",
			code:      http.StatusOK,
			floor:     5 * time.Second,
			ceil:      6 * time.Second,
		},
		"watch request without timeout": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes?watch=true",
			code:      http.StatusOK,
			floor:     10 * time.Second,
			ceil:      20 * time.Second,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var start, end time.Time
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				start = time.Now()
				lp.ServeHTTP(w, req)
				end = time.Now()
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			if tt.floor.Seconds() != 0 {
				if start.Add(tt.floor).After(end) {
					t.Errorf("exec time is less than floor time %v", tt.floor)
				}
			}

			if start.Add(tt.ceil).Before(end) {
				t.Errorf("exec time is more than ceil time %v", tt.ceil)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}
func TestServeHTTPForPost(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent string
		accept    string
		verb      string
		path      string
		data      string
		code      int
	}{
		"post request": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "POST",
			path:      "/api/v1/nodes/mynode",
			data:      "test for post node",
			code:      http.StatusCreated,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			buf := bytes.NewBufferString(tt.data)
			req, _ := http.NewRequest(tt.verb, tt.path, buf)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.data) != 0 {
				req.Header.Set("Content-Length", fmt.Sprintf("%d", len(tt.data)))
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				lp.ServeHTTP(w, req)
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			rbuf := bytes.NewBuffer([]byte{})
			_, _ = rbuf.ReadFrom(result.Body)
			if rbuf.String() != tt.data {
				t.Errorf("got response body %s, but expect body is %s", rbuf.String(), tt.data)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestServeHTTPForDelete(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent string
		accept    string
		verb      string
		path      string
		code      int
	}{
		"delete request": {
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "DELETE",
			path:      "/api/v1/nodes/mynode",
			code:      http.StatusForbidden,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				lp.ServeHTTP(w, req)
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestServeHTTPForGetReqCache(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, nil, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent    string
		keyBuildInfo storage.KeyBuildInfo
		inputObj     []runtime.Object
		accept       string
		verb         string
		path         string
		resource     string
		code         int
		data         struct {
			ns   string
			name string
			rv   string
		}
	}{
		"get pod request": {
			userAgent: "kubelet",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []runtime.Object{
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod1",
						Namespace:       "default",
						ResourceVersion: "1",
					},
				},
			},
			accept:   "application/json",
			verb:     "GET",
			path:     "/api/v1/namespaces/default/pods/mypod1",
			resource: "pods",
			code:     http.StatusOK,
			data: struct {
				ns   string
				name string
				rv   string
			}{
				ns:   "default",
				name: "mypod1",
				rv:   "1",
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerM.CreateSerializer(tt.accept, "", "v1", tt.resource)
			accessor := meta.NewAccessor()
			for i := range tt.inputObj {
				name, _ := accessor.Name(tt.inputObj[i])
				tt.keyBuildInfo.Name = name
				key, err := sWrapper.KeyFunc(tt.keyBuildInfo)
				if err != nil {
					t.Errorf("failed to get key of obj, %v", err)
				}
				err = sWrapper.Create(key, tt.inputObj[i])
				if err != nil {
					t.Errorf("failed to create obj in storage, %v", err)
				}
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				lp.ServeHTTP(w, req)
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			buf := bytes.NewBuffer([]byte{})
			_, err = buf.ReadFrom(result.Body)
			if err != nil {
				t.Errorf("read from result body failed, %v", err)
			}

			obj, err := s.Decode(buf.Bytes())
			if err != nil {
				t.Errorf("decode response failed, %v", err)
			}

			pod, ok := obj.(*v1.Pod)
			if !ok {
				t.Errorf("Expect v1.Pod object, but not %v", obj)
			}

			if pod.Namespace != tt.data.ns {
				t.Errorf("Got ns %s, but expect ns %s", pod.Namespace, tt.data.ns)
			}

			if pod.Name != tt.data.name {
				t.Errorf("Got name %s, but expect name %s", pod.Name, tt.data.name)
			}

			if pod.ResourceVersion != tt.data.rv {
				t.Errorf("Got rv %s, but expect rv %s", pod.ResourceVersion, tt.data.rv)
			}

			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestServeHTTPForListReqCache(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	restRESTMapperMgr, _ := hubmeta.NewRESTMapperManager(rootDir)
	cacheM := cachemanager.NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	fn := func() bool {
		return false
	}

	lp := NewLocalProxy(cacheM, fn, fn, 0)

	testcases := map[string]struct {
		userAgent    string
		keyBuildInfo storage.KeyBuildInfo
		preCachedObj []runtime.Object
		accept       string
		verb         string
		path         string
		resource     string
		gvr          schema.GroupVersionResource
		code         int
		expectD      struct {
			rv   string
			data map[string]struct{}
		}
	}{
		"list pods request": {
			userAgent: "kubelet",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			preCachedObj: []runtime.Object{
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod2",
						Namespace:       "default",
						ResourceVersion: "3",
					},
				},
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod3",
						Namespace:       "default",
						ResourceVersion: "5",
					},
				},
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod4",
						Namespace:       "default",
						ResourceVersion: "6",
					},
				},
			},
			accept:   "application/json",
			verb:     "GET",
			path:     "/api/v1/namespaces/default/pods",
			resource: "pods",
			code:     http.StatusOK,
			expectD: struct {
				rv   string
				data map[string]struct{}
			}{
				rv: "6",
				data: map[string]struct{}{
					"default-mypod2-3": {},
					"default-mypod3-5": {},
					"default-mypod4-6": {},
				},
			},
		},
		"list unregistered resource(Foo) request": {
			userAgent: "kubelet",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			preCachedObj: []runtime.Object{},
			accept:       "application/json",
			verb:         "GET",
			path:         "/api/samplecontroller.k8s.io/v1/foos",
			resource:     "foos",
			gvr:          schema.GroupVersionResource{Group: "samplecontroller.k8s.io", Version: "v1", Resource: "foos"},
			code:         http.StatusNotFound,
			expectD: struct {
				rv   string
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerM.CreateSerializer(tt.accept, "", "v1", tt.resource)
			accessor := meta.NewAccessor()
			for i := range tt.preCachedObj {
				name, _ := accessor.Name(tt.preCachedObj[i])
				tt.keyBuildInfo.Name = name
				key, err := sWrapper.KeyFunc(tt.keyBuildInfo)
				if err != nil {
					t.Errorf("failed to get key of obj, %v", err)
				}
				err = sWrapper.Create(key, tt.preCachedObj[i])
				if err != nil {
					t.Errorf("failed to create obj in storage, %v", err)
				}
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				lp.ServeHTTP(w, req)
			})

			handler = proxyutil.WithRequestClientComponent(handler, util.WorkingModeEdge)
			handler = proxyutil.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)

			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			// For unregistered resources, server should return 404 not found
			if result.StatusCode == http.StatusNotFound {
				if _, gvk := restRESTMapperMgr.KindFor(tt.gvr); !gvk.Empty() {
					t.Errorf("this resources %v is registered, but it should return 404 for unregistered resource", tt.gvr)
				}
				return
			}
			if result.StatusCode != tt.code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.code)
			}

			buf := bytes.NewBuffer([]byte{})
			_, err = buf.ReadFrom(result.Body)
			if err != nil {
				t.Errorf("read from result body failed, %v", err)
			}

			obj, err := s.Decode(buf.Bytes())
			if err != nil {
				t.Errorf("decode response failed, %v", err)
			}

			list, ok := obj.(*v1.PodList)
			if !ok {
				t.Errorf("Expect v1.PodList object, but not, %v", obj)
			}

			if list.ResourceVersion != tt.expectD.rv {
				t.Errorf("Got list rv %s, but expect list rv %s", list.ResourceVersion, tt.expectD.rv)
			}

			if len(list.Items) != len(tt.expectD.data) {
				t.Errorf("Got %d pods, but exepect %d pods", len(list.Items), len(tt.expectD.data))
			}

			for i := range list.Items {
				key := fmt.Sprintf("%s-%s-%s", list.Items[i].Namespace, list.Items[i].Name, list.Items[i].ResourceVersion)
				if _, ok := tt.expectD.data[key]; !ok {
					t.Errorf("Got pod %s, but not expect pod", key)
				}
			}

			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}
