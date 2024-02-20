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

package cachemanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	nodev1beta1 "k8s.io/api/node/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	rootDir                   = "/tmp/cache-manager"
	fakeClient                = fake.NewSimpleClientset()
	fakeSharedInformerFactory = informers.NewSharedInformerFactory(fakeClient, 0)
)

func TestCacheGetResponse(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to create RESTMapper manager, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	testcases := map[string]struct {
		group        string
		version      string
		keyBuildInfo storage.KeyBuildInfo
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		resource     string
		namespaced   bool
		expectResult struct {
			err  error
			rv   string
			name string
			ns   string
			kind string
		}
		cacheResponseErr bool
	}{
		"cache response for pod with not assigned node": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod1",
					Namespace:       "default",
					ResourceVersion: "1",
				},
			}),
			userAgent:        "kubelet",
			accept:           "application/json",
			verb:             "GET",
			path:             "/api/v1/namespaces/default/pods/mypod1",
			resource:         "pods",
			namespaced:       true,
			cacheResponseErr: true,
		},
		"cache response for get pod": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod1",
					Namespace:       "default",
					ResourceVersion: "1",
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod1",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "mypod1",
				ns:   "default",
				kind: "Pod",
			},
		},
		"cache response for get pod2": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod2",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod2",
					Namespace:       "default",
					ResourceVersion: "3",
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod2",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "3",
				name: "mypod2",
				ns:   "default",
				kind: "Pod",
			},
		},
		"cache response for get node": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "mynode1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mynode1",
					ResourceVersion: "4",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes/mynode1",
			resource:   "nodes",
			namespaced: false,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "4",
				name: "mynode1",
				kind: "Node",
			},
		},
		"cache response for get node2": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "mynode2",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mynode2",
					ResourceVersion: "6",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes/mynode2",
			resource:   "nodes",
			namespaced: false,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "6",
				name: "mynode2",
				kind: "Node",
			},
		},
		//used to test whether custom resources can be cached correctly
		"cache response for get crontab": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab1",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab1",
						"namespace":       "default",
						"resourceVersion": "1",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "crontab1",
				ns:   "default",
				kind: "CronTab",
			},
		},
		"cache response for get crontab2": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab2",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab2",
						"namespace":       "default",
						"resourceVersion": "3",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab2",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "3",
				name: "crontab2",
				ns:   "default",
				kind: "CronTab",
			},
		},
		"cache response for get foo without namespace": {
			group:   "samplecontroller.k8s.io",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo1",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo1",
						"resourceVersion": "3",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo1",
			resource:   "foos",
			namespaced: false,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "3",
				name: "foo1",
				kind: "Foo",
			},
		},
		"cache response for get foo2 without namespace": {
			group:   "samplecontroller.k8s.io",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo2",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo2",
						"resourceVersion": "5",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo2",
			resource:   "foos",
			namespaced: false,
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "5",
				name: "foo2",
				kind: "Foo",
			},
		},
		"cache response for Status": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "test",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&metav1.Status{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Status",
				},
				Status:  "Failure",
				Message: "node test is not exist",
				Reason:  "NotFound",
				Code:    404,
			}),
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/nodes/test",
			resource:  "nodes",
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				err: storage.ErrStorageNotFound,
			},
		},
		"cache response for nil object": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "test",
				Group:     "",
				Version:   "v1",
			},
			inputObj:         nil,
			userAgent:        "kubelet",
			accept:           "application/json",
			verb:             "GET",
			path:             "/api/v1/nodes/test",
			resource:         "nodes",
			cacheResponseErr: true,
		},
		"cache response for get namespace": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "namespaces",
				Name:      "kube-system",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kube-system",
					ResourceVersion: "1",
				},
			}),
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/kube-system",
			resource:  "namespaces",
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "kube-system",
				kind: "Namespace",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerM.CreateSerializer(tt.accept, tt.group, tt.version, tt.resource)
			encoder, err := s.Encoder(tt.accept, nil)
			if err != nil {
				t.Fatalf("could not create encoder, %v", err)
			}

			buf := bytes.NewBuffer([]byte{})
			if tt.inputObj != nil {
				err = encoder.Encode(tt.inputObj, buf)
				if err != nil {
					t.Fatalf("could not encode input object, %v", err)
				}
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				ctx = util.WithRespContentType(ctx, tt.accept)
				req = req.WithContext(ctx)
				prc := io.NopCloser(buf)
				err = yurtCM.CacheResponse(req, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.cacheResponseErr && err == nil {
				t.Errorf("expect err, but do not get error")
			} else if !tt.cacheResponseErr && err != nil {
				t.Errorf("expect no err, but got error %v", err)
			}

			if len(tt.expectResult.name) == 0 {
				return
			}
			key, err := sWrapper.KeyFunc(tt.keyBuildInfo)
			if err != nil {
				t.Errorf("failed to create key, %v", err)
			}
			obj, err := sWrapper.Get(key)
			if err != nil || obj == nil {
				if !errors.Is(tt.expectResult.err, err) {
					t.Errorf("expect get error %v, but got %v", tt.expectResult.err, err)
				}
				t.Logf("get expected err %v for key %s", tt.expectResult.err, tt.keyBuildInfo)
			} else {
				name, _ := accessor.Name(obj)
				rv, _ := accessor.ResourceVersion(obj)
				kind, _ := accessor.Kind(obj)
				if tt.expectResult.name != name {
					t.Errorf("Got name %s, but expect name %s", name, tt.expectResult.name)
				}

				if tt.expectResult.rv != rv {
					t.Errorf("Got rv %s, but expect rv %s", rv, tt.expectResult.rv)
				}

				if tt.namespaced {
					ns, _ := accessor.Namespace(obj)
					if tt.expectResult.ns != ns {
						t.Errorf("Got ns %s, but expect ns %s", ns, tt.expectResult.ns)
					}
				}

				if tt.expectResult.kind != kind {
					t.Errorf("Got kind %s, but expect kind %s", kind, tt.expectResult.kind)
				}
				t.Logf("get key %s successfully", tt.keyBuildInfo)
			}

			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}
			if err = restRESTMapperMgr.ResetRESTMapper(); err != nil {
				t.Errorf("failed to delete cached DynamicRESTMapper, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestCacheWatchResponse(t *testing.T) {
	mkPod := func(id string, rv string) *v1.Pod {
		return &v1.Pod{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: "default", ResourceVersion: rv},
			Spec:       v1.PodSpec{NodeName: "node1"},
		}
	}

	//used to generate the custom resources
	mkCronTab := func(id string, rv string) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "stable.example.com/v1",
				"kind":       "CronTab",
				"metadata": map[string]interface{}{
					"name":            id,
					"namespace":       "default",
					"resourceVersion": rv,
				},
			},
		}
	}

	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to create RESTMapper manager, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	testcases := map[string]struct {
		group        string
		version      string
		keyBuildInfo storage.KeyBuildInfo
		inputObj     []watch.Event
		userAgent    string
		accept       string
		verb         string
		path         string
		resource     string
		namespaced   bool
		expectResult struct {
			err  bool
			data map[string]struct{}
		}
	}{
		"add pods": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Added, Object: mkPod("mypod2", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"pod-default-mypod1-2": {},
					"pod-default-mypod2-4": {},
					"pod-default-mypod3-6": {},
				},
			},
		},
		"add and delete pods": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Deleted, Object: mkPod("mypod1", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			namespaced: true,
			resource:   "pods",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"pod-default-mypod3-6": {},
				},
			},
		},
		"add and update pods": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"pod-default-mypod1-4": {},
					"pod-default-mypod3-6": {},
				},
			},
		},
		"not update pods": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "6")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Modified, Object: mkPod("mypod1", "2")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"pod-default-mypod1-6": {},
				},
			},
		},
		//used to test whether custom resource's watch-events can be cached correctly
		"cache response for watch add crontabs": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Added, Object: mkCronTab("crontab2", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"crontab-default-crontab1-2": {},
					"crontab-default-crontab2-4": {},
					"crontab-default-crontab3-6": {},
				},
			},
		},
		"cache response for watch add and delete crontabs": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Deleted, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"crontab-default-crontab3-6": {},
				},
			},
		},
		"cache response for watch add and update crontabs": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"crontab-default-crontab1-4": {},
					"crontab-default-crontab3-6": {},
				},
			},
		},
		"cache response for watch not update crontabs": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "6")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "2")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"crontab-default-crontab1-6": {},
				},
			},
		},
		"should not return error when storing bookmark watch event": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []watch.Event{
				{Type: watch.Bookmark, Object: mkPod("mypod1", "2")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerM.CreateSerializer(tt.accept, tt.group, tt.version, tt.resource)
			r, w := io.Pipe()
			go func(w *io.PipeWriter) {
				//For unregistered GVKs, the normal encoding is used by default and the original GVK information is set

				for i := range tt.inputObj {
					if _, err := s.WatchEncode(w, &tt.inputObj[i]); err != nil {
						t.Errorf("%d: encode watch unexpected error: %v", i, err)
						continue
					}
					time.Sleep(100 * time.Millisecond)
				}
				w.Close()
			}(w)

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}
			req.RemoteAddr = "127.0.0.1"

			var err error
			rc := io.NopCloser(r)
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				ctx = util.WithRespContentType(ctx, tt.accept)
				req = req.WithContext(ctx)
				err = yurtCM.CacheResponse(req, rc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err && err == nil {
				t.Errorf("expect err, but do not got err")
			} else if err != nil && err != io.EOF {
				t.Errorf("failed to cache resposne, %v", err)
			}

			if len(tt.expectResult.data) == 0 {
				return
			}

			rootKey, err := sWrapper.KeyFunc(tt.keyBuildInfo)
			if err != nil {
				t.Errorf("failed to get key, %v", err)
			}
			objs, err := sWrapper.List(rootKey)
			if err != nil || len(objs) == 0 {
				t.Errorf("failed to get object from storage")
			}

			if !compareObjectsAndKeys(t, objs, tt.namespaced, tt.expectResult.data) {
				t.Errorf("got unexpected objects for keys for watch request")
			}

			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}
			if err = restRESTMapperMgr.ResetRESTMapper(); err != nil {
				t.Errorf("failed to delete cached DynamicRESTMapper, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestCacheListResponse(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)

	serializerM := serializer.NewSerializerManager()
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to create RESTMapper manager, %v", err)
	}
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	testcases := map[string]struct {
		group        string
		version      string
		keyBuildInfo storage.KeyBuildInfo
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		resource     string
		namespaced   bool
		expectResult struct {
			err  bool
			data map[string]struct{}
		}
	}{
		"list pods": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&v1.PodList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "PodList",
					},
					ListMeta: metav1.ListMeta{
						ResourceVersion: "5",
					},
					Items: []v1.Pod{
						{
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
						{
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
						{
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
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods",
			resource:   "pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"pod-default-mypod1-1": {},
					"pod-default-mypod2-3": {},
					"pod-default-mypod3-5": {},
				},
			},
		},
		"list nodes": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&v1.NodeList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "NodeList",
					},
					ListMeta: metav1.ListMeta{
						ResourceVersion: "12",
					},
					Items: []v1.Node{
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Node",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "mynode1",
								ResourceVersion: "6",
							},
						},
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Node",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "mynode2",
								ResourceVersion: "8",
							},
						},
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Node",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "mynode3",
								ResourceVersion: "10",
							},
						},
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Node",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "mynode4",
								ResourceVersion: "12",
							},
						},
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes",
			resource:   "nodes",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"node-mynode1-6":  {},
					"node-mynode2-8":  {},
					"node-mynode3-10": {},
					"node-mynode4-12": {},
				},
			},
		},
		"list nodes with fieldselector": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&v1.NodeList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "NodeList",
					},
					ListMeta: metav1.ListMeta{
						ResourceVersion: "12",
					},
					Items: []v1.Node{
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Node",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "mynode",
								ResourceVersion: "12",
							},
						},
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes?fieldselector=meatadata.name=mynode",
			resource:   "nodes",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"node-mynode-12": {},
				},
			},
		},
		"list runtimeclasses with no objects": {
			group:   "node.k8s.io",
			version: "v1beta1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "runtimeclasses",
				Group:     "node.k8s.io",
				Version:   "v1beta1",
			},
			inputObj: runtime.Object(
				&nodev1beta1.RuntimeClassList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "node.k8s.io/v1beta1",
						Kind:       "RuntimeClassList",
					},
					ListMeta: metav1.ListMeta{
						ResourceVersion: "12",
					},
					Items: []nodev1beta1.RuntimeClass{},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/node.k8s.io/v1beta1/runtimeclasses",
			resource:   "runtimeclasses",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list with status": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodetest",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&metav1.Status{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Status",
					},
					Status:  "Failure",
					Message: "nodetest is not exist",
					Reason:  "NotFound",
					Code:    404,
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/node",
			resource:   "nodes",
			namespaced: false,
		},
		//used to test whether custom resource list can be cached correctly
		"cache response for list crontabs": {
			group:   "stable.example.com",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"apiVersion": "stable.example.com/v1",
						"kind":       "CronTabList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "2",
							"selfLink":        "/apis/stable.example.com/v1/namespaces/default/crontabs",
						},
					},
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"apiVersion": "stable.example.com/v1",
								"kind":       "CronTab",
								"metadata": map[string]interface{}{
									"name":            "crontab1",
									"namespace":       "default",
									"resourceVersion": "1",
								},
							},
						},
						{
							Object: map[string]interface{}{
								"apiVersion": "stable.example.com/v1",
								"kind":       "CronTab",
								"metadata": map[string]interface{}{
									"name":            "crontab2",
									"namespace":       "default",
									"resourceVersion": "2",
								},
							},
						},
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs",
			resource:   "crontabs",
			namespaced: true,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"crontab-default-crontab1-1": {},
					"crontab-default-crontab2-2": {},
				},
			},
		},
		"cache response for list foos without namespace": {
			group:   "samplecontroller.k8s.io",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"apiVersion": "samplecontroller.k8s.io/v1",
						"kind":       "FooList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "2",
							"selfLink":        "/apis/samplecontroller.k8s.io/v1/foos",
						},
					},
					Items: []unstructured.Unstructured{
						{
							Object: map[string]interface{}{
								"apiVersion": "samplecontroller.k8s.io/v1",
								"kind":       "Foo",
								"metadata": map[string]interface{}{
									"name":            "foo1",
									"resourceVersion": "1",
								},
							},
						},
						{
							Object: map[string]interface{}{
								"apiVersion": "samplecontroller.k8s.io/v1",
								"kind":       "Foo",
								"metadata": map[string]interface{}{
									"name":            "foo2",
									"resourceVersion": "2",
								},
							},
						},
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos",
			resource:   "foos",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"foo-foo1-1": {},
					"foo-foo2-2": {},
				},
			},
		},
		"list foos with no objects": {
			group:   "samplecontroller.k8s.io",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"apiVersion": "samplecontroller.k8s.io/v1",
						"kind":       "FooList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "2",
							"selfLink":        "/apis/samplecontroller.k8s.io/v1/foos",
						},
					},
					Items: []unstructured.Unstructured{},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos",
			resource:   "foos",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list namespaces": {
			group:   "",
			version: "v1",
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "namespaces",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(
				&v1.NamespaceList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "NamespaceList",
					},
					ListMeta: metav1.ListMeta{
						ResourceVersion: "3",
					},
					Items: []v1.Namespace{
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Namespace",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "kube-system",
								ResourceVersion: "4",
							},
						},
						{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "v1",
								Kind:       "Namespace",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:            "default",
								ResourceVersion: "5",
							},
						},
					},
				},
			),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces",
			resource:   "namespaces",
			namespaced: false,
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"namespace-kube-system-4": {},
					"namespace-default-5":     {},
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerM.CreateSerializer(tt.accept, tt.group, tt.version, tt.resource)
			encoder, err := s.Encoder(tt.accept, nil)
			if err != nil {
				t.Fatalf("could not create encoder, %v", err)
			}

			buf := bytes.NewBuffer([]byte{})
			err = encoder.Encode(tt.inputObj, buf)
			if err != nil {
				t.Fatalf("could not encode input object, %v", err)
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}
			req.RemoteAddr = "127.0.0.1"

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				ctx = util.WithRespContentType(ctx, tt.accept)
				req = req.WithContext(ctx)
				prc := io.NopCloser(buf)
				err = yurtCM.CacheResponse(req, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err {
				if err == nil {
					t.Error("Got no error, but expect err")
				}
			} else {
				if err != nil {
					t.Errorf("Got error %v", err)
				}

				rootKey, err := sWrapper.KeyFunc(tt.keyBuildInfo)
				if err != nil {
					t.Errorf("failed to get key, %v", err)
				}
				objs, err := sWrapper.List(rootKey)
				if err != nil {
					// If error is storage.ErrStorageNotFound, it means that no object is cached in the hard disk
					if errors.Is(err, storage.ErrStorageNotFound) {
						if tt.expectResult.data != nil {
							t.Errorf("expect %v objects, but get nil.", len(tt.expectResult.data))
						}
					} else {
						t.Errorf("got unexpected error %v", err)
					}
				}

				if !compareObjectsAndKeys(t, objs, tt.namespaced, tt.expectResult.data) {
					t.Errorf("got unexpected objects for keys")
				}
			}
			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}
			if err = restRESTMapperMgr.ResetRESTMapper(); err != nil {
				t.Errorf("failed to delete cached DynamicRESTMapper, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func TestQueryCacheForGet(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to create RESTMapper manager, %v", err)
	}
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	testcases := map[string]struct {
		keyBuildInfo storage.KeyBuildInfo
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		namespaced   bool
		expectResult struct {
			err  bool
			rv   string
			name string
			ns   string
			kind string
		}
	}{
		"no client": {
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod1",
			namespaced: true,
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod1",
				Group:     "",
				Version:   "v1",
			},
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"not resource request": {
			accept:     "application/json",
			verb:       "GET",
			path:       "/healthz",
			namespaced: true,
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "healthz",
			},
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"post pod": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod1",
					Namespace:       "default",
					ResourceVersion: "1",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "POST",
			path:       "/api/v1/namespaces/default/pods",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"get pod": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod1",
					Namespace:       "default",
					ResourceVersion: "1",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod1",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "mypod1",
				ns:   "default",
				kind: "Pod",
			},
		},
		"update pod": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Name:      "mypod2",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod2",
					Namespace:       "default",
					ResourceVersion: "2",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PUT",
			path:       "/api/v1/namespaces/default/pods/mypod2",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "2",
				name: "mypod2",
				ns:   "default",
				kind: "Pod",
			},
		},
		"update node": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "mynode1",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mynode1",
					ResourceVersion: "3",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PUT",
			path:       "/api/v1/nodes/mynode1",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "3",
				name: "mynode1",
				kind: "Node",
			},
		},
		"patch node": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Name:      "mynode2",
				Group:     "",
				Version:   "v1",
			},
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mynode2",
					ResourceVersion: "4",
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PATCH",
			path:       "/api/v1/nodes/mynode2/status",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "4",
				name: "mynode2",
				kind: "Node",
			},
		},

		//used to test whether the query local Custom Resource request can be handled correctly
		"no client for crontab": {
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
			namespaced: true,
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab1",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"query post crontab": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab1",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab1",
						"namespace":       "default",
						"resourceVersion": "1",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "POST",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"query get crontab": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab1",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab1",
						"namespace":       "default",
						"resourceVersion": "1",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "crontab1",
				ns:   "default",
				kind: "CronTab",
			},
		},
		"query update crontab": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab2",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab2",
						"namespace":       "default",
						"resourceVersion": "2",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PUT",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab2",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "2",
				name: "crontab2",
				ns:   "default",
				kind: "CronTab",
			},
		},
		"query patch crontab": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Name:      "crontab3",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "stable.example.com/v1",
					"kind":       "CronTab",
					"metadata": map[string]interface{}{
						"name":            "crontab3",
						"namespace":       "default",
						"resourceVersion": "4",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PATCH",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab3/status",
			namespaced: true,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "4",
				name: "crontab3",
				ns:   "default",
				kind: "CronTab",
			},
		},
		"query post foo": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo1",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo1",
						"resourceVersion": "1",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "POST",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo1",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				err: true,
			},
		},
		"query get foo": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo1",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo1",
						"resourceVersion": "1",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo1",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "1",
				name: "foo1",
				kind: "Foo",
			},
		},
		"query update foo": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo2",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo2",
						"resourceVersion": "2",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PUT",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo2",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "2",
				name: "foo2",
				kind: "Foo",
			},
		},
		"query patch foo": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Name:      "foo3",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			inputObj: runtime.Object(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "samplecontroller.k8s.io/v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":            "foo3",
						"resourceVersion": "4",
					},
				},
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "PATCH",
			path:       "/apis/samplecontroller.k8s.io/v1/foos/foo3/status",
			namespaced: false,
			expectResult: struct {
				err  bool
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "4",
				name: "foo3",
				kind: "Foo",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			var err error
			key, err := sWrapper.KeyFunc(tt.keyBuildInfo)
			if err != nil {
				t.Errorf("failed to get key with info %v, %v", tt.keyBuildInfo, err)
			}
			_ = sWrapper.Create(key, tt.inputObj)
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			req.RemoteAddr = "127.0.0.1"

			var name, ns, rv, kind string
			var obj runtime.Object
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				obj, err = yurtCM.QueryCache(req)
				if err == nil {
					name, _ = accessor.Name(obj)
					rv, _ = accessor.ResourceVersion(obj)
					ns, _ = accessor.Namespace(obj)
					kind, _ = accessor.Kind(obj)
				}
			})

			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err {
				if err == nil {
					t.Errorf("Got no error, but expect err")
				}
			} else {
				if err != nil {
					t.Errorf("Got error %v", err)
				}

				if tt.expectResult.name != name {
					t.Errorf("Got name %s, but expect name %s", name, tt.expectResult.name)
				}

				if tt.expectResult.rv != rv {
					t.Errorf("Got rv %s, but expect rv %s", rv, tt.expectResult.rv)
				}

				if tt.namespaced {
					if tt.expectResult.ns != ns {
						t.Errorf("Got ns %s, but expect ns %s", ns, tt.expectResult.ns)
					}
				}

				if tt.expectResult.kind != kind {
					t.Errorf("Got kind %s, but expect kind %s", kind, tt.expectResult.kind)
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

// func TestQueryCacheForGetClusterInfo(t *testing.T) {
// 	dStorage, err := disk.NewDiskStorage(rootDir)
// 	if err != nil {
// 		t.Errorf("failed to create disk storage, %v", err)
// 	}
// 	sWrapper := NewStorageWrapper(dStorage)
// 	serializerM := serializer.NewSerializerManager()
// 	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
// 	if err != nil {
// 		t.Errorf("failed to create RESTMapper manager, %v", err)
// 	}
// 	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

// 	testcases := map[string]struct {
// 		path         string
// 		inputInfo    []byte
// 		expectResult struct {
// 			expectInfo []byte
// 			expectErr  error
// 		}
// 	}{
// 		"query version info": {
// 			path:      "/version",
// 			inputInfo: []byte(versionBytes),
// 			expectResult: struct {
// 				expectInfo []byte
// 				expectErr  error
// 			}{
// 				expectInfo: []byte(versionBytes),
// 				expectErr:  nil,
// 			},
// 		},
// 		"query version info with parameters": {
// 			path:      "/version?timeout=32s",
// 			inputInfo: []byte(versionBytes),
// 			expectResult: struct {
// 				expectInfo []byte
// 				expectErr  error
// 			}{
// 				expectInfo: []byte(versionBytes),
// 				expectErr:  nil,
// 			},
// 		},
// 		"query version info not existing": {
// 			path:      "/version",
// 			inputInfo: nil,
// 			expectResult: struct {
// 				expectInfo []byte
// 				expectErr  error
// 			}{
// 				expectInfo: nil,
// 				expectErr:  storage.ErrStorageNotFound,
// 			},
// 		},
// 		"query unknown ClusterInfoType": {
// 			path:      "/any-path",
// 			inputInfo: nil,
// 			expectResult: struct {
// 				expectInfo []byte
// 				expectErr  error
// 			}{
// 				expectInfo: nil,
// 				expectErr:  storage.ErrUnknownClusterInfoType,
// 			},
// 		},
// 	}

// 	resolver := newTestRequestInfoResolver()
// 	for k, tt := range testcases {
// 		t.Run(k, func(t *testing.T) {
// 			var err error
// 			req, _ := http.NewRequest("GET", tt.path, nil)
// 			var buf []byte
// 			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				buf, err = yurtCM.QueryClusterInfoFromCache(req)
// 			})
// 			handler = filters.WithRequestInfo(handler, resolver)
// 			if tt.inputInfo != nil {
// 				sWrapper.SaveClusterInfo()
// 			}
// 		})
// 	}
// }

func TestQueryCacheForList(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)
	serializerM := serializer.NewSerializerManager()
	restRESTMapperMgr, err := hubmeta.NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to create RESTMapper manager, %v", err)
	}
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

	testcases := map[string]struct {
		keyBuildInfo storage.KeyBuildInfo
		cachedKind   string
		inputObj     []runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		namespaced   bool
		expectResult struct {
			err      bool
			queryErr error
			rv       string
			data     map[string]struct{}
		}
	}{
		"list with no user agent": {
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods",
			namespaced: true,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err: true,
			},
		},
		"list pods": {
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
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod2",
						Namespace:       "default",
						ResourceVersion: "2",
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
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods",
			namespaced: true,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				rv: "5",
				data: map[string]struct{}{
					"pod-default-mypod1-1": {},
					"pod-default-mypod2-2": {},
					"pod-default-mypod3-5": {},
				},
			},
		},
		"list nodes": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Group:     "",
				Version:   "v1",
			},
			inputObj: []runtime.Object{
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode1",
						ResourceVersion: "6",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode2",
						ResourceVersion: "8",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode3",
						ResourceVersion: "10",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode4",
						ResourceVersion: "12",
					},
				},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				rv: "12",
				data: map[string]struct{}{
					"node-mynode1-6":  {},
					"node-mynode2-8":  {},
					"node-mynode3-10": {},
					"node-mynode4-12": {},
				},
			},
		},
		"list runtimeclass": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "runtimeclasses",
				Group:     "node.k8s.io",
				Version:   "v1beta1",
			},
			inputObj: []runtime.Object{
				&unstructured.Unstructured{},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/node.k8s.io/v1beta1/runtimeclasses",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list pods of one namespace and no pods of this namespace in cache": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Group:     "",
				Version:   "v1",
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/pods/default",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err:      true,
				queryErr: storage.ErrStorageNotFound,
			},
		},

		//used to test whether the query local Custom Resource list request can be handled correctly
		"list crontabs": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "crontabs",
				Namespace: "default",
				Group:     "stable.example.com",
				Version:   "v1",
			},
			cachedKind: "stable.example.com/v1/CronTab",
			inputObj: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "stable.example.com/v1",
						"kind":       "CronTab",
						"metadata": map[string]interface{}{
							"name":            "crontab1",
							"namespace":       "default",
							"resourceVersion": "1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "stable.example.com/v1",
						"kind":       "CronTab",
						"metadata": map[string]interface{}{
							"name":            "crontab2",
							"namespace":       "default",
							"resourceVersion": "2",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "stable.example.com/v1",
						"kind":       "CronTab",
						"metadata": map[string]interface{}{
							"name":            "crontab3",
							"namespace":       "default",
							"resourceVersion": "5",
						},
					},
				},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs",
			namespaced: true,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				rv: "5",
				data: map[string]struct{}{
					"crontab-default-crontab1-1": {},
					"crontab-default-crontab2-2": {},
					"crontab-default-crontab3-5": {},
				},
			},
		},
		"list foos": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			cachedKind: "samplecontroller.k8s.io/v1/Foo",
			inputObj: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "samplecontroller.k8s.io/v1",
						"kind":       "Foo",
						"metadata": map[string]interface{}{
							"name":            "foo1",
							"resourceVersion": "1",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "samplecontroller.k8s.io/v1",
						"kind":       "Foo",
						"metadata": map[string]interface{}{
							"name":            "foo2",
							"resourceVersion": "2",
						},
					},
				},
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "samplecontroller.k8s.io/v1",
						"kind":       "Foo",
						"metadata": map[string]interface{}{
							"name":            "foo3",
							"resourceVersion": "5",
						},
					},
				},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				rv: "5",
				data: map[string]struct{}{
					"foo-foo1-1": {},
					"foo-foo2-2": {},
					"foo-foo3-5": {},
				},
			},
		},
		"list foos with no objs": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "foos",
				Group:     "samplecontroller.k8s.io",
				Version:   "v1",
			},
			cachedKind: "samplecontroller.k8s.io/v1/Foo",
			inputObj: []runtime.Object{
				&unstructured.Unstructured{},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/samplecontroller.k8s.io/v1/foos",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list unregistered resources": {
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/sample.k8s.io/v1/abcs",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err:      true,
				queryErr: hubmeta.ErrGVRNotRecognized,
			},
		},
		"list resources not exist": {
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/nodes",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err:      true,
				queryErr: storage.ErrStorageNotFound,
			},
		},
		"list non-existing resource with metadata.name fieldSelector": {
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/kube-system/configmaps?fieldSelector=metadata.name%3Dkubernetes-services-endpoint",
			namespaced: false,
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err:  false,
				data: map[string]struct{}{},
			},
		},
		"list existing resource with metadata.name fieldSelector": {
			keyBuildInfo: storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Group:     "",
				Version:   "v1",
				Namespace: "default",
				Name:      "nginx",
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?fieldSelector=metadata.name%3Dnginx",
			namespaced: true,
			inputObj: []runtime.Object{
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "nginx",
						Namespace:       "default",
						ResourceVersion: "5",
					},
				},
			},
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				err: false,
				rv:  "5",
				data: map[string]struct{}{
					"pod-default-nginx-5": {},
				},
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			for i := range tt.inputObj {
				v, _ := accessor.Name(tt.inputObj[i])
				tt.keyBuildInfo.Name = v
				key, err := sWrapper.KeyFunc(tt.keyBuildInfo)
				if err != nil {
					t.Errorf("failed to get key, %v", err)
				}
				_ = sWrapper.Create(key, tt.inputObj[i])
			}

			// It is used to simulate caching GVK information. If the caching is successful,
			// the next process can obtain the correct GVK information when constructing an empty List.
			if tt.cachedKind != "" {
				info := strings.Split(tt.cachedKind, hubmeta.SepForGVR)
				gvk := schema.GroupVersionKind{
					Group:   info[0],
					Version: info[1],
					Kind:    info[2],
				}
				_ = restRESTMapperMgr.UpdateKind(gvk)
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			req.RemoteAddr = "127.0.0.1"

			items := make([]runtime.Object, 0)
			var rv string
			var err error
			var list runtime.Object
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				list, err = yurtCM.QueryCache(req)
				if err == nil {
					listMetaInterface, err := meta.ListAccessor(list)
					if err != nil {
						t.Errorf("failed to access list obj, %v", err)
					}
					rv = listMetaInterface.GetResourceVersion()
					items, _ = meta.ExtractList(list)
				}
			})

			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err {
				if err == nil {
					t.Errorf("Got no error, but expect err")
				}

				if tt.expectResult.queryErr != nil && !errors.Is(tt.expectResult.queryErr, err) {
					t.Errorf("expect err %v, but got %v", tt.expectResult.queryErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Got error %v", err)
				}

				if tt.expectResult.rv != "" && tt.expectResult.rv != rv {
					t.Errorf("Got rv %s, but expect rv %s", rv, tt.expectResult.rv)
				}

				if !compareObjectsAndKeys(t, items, tt.namespaced, tt.expectResult.data) {
					t.Errorf("got unexpected objects for keys")
				}
			}
			err = sWrapper.DeleteComponentResources("kubelet")
			if err != nil {
				t.Errorf("failed to delete collection: kubelet, %v", err)
			}

			if err = restRESTMapperMgr.ResetRESTMapper(); err != nil {
				t.Errorf("failed to delete cached DynamicRESTMapper, %v", err)
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func compareObjectsAndKeys(t *testing.T, objs []runtime.Object, namespaced bool, keys map[string]struct{}) bool {
	if len(objs) != len(keys) {
		t.Errorf("expect %d keys, but got %d objects", len(keys), len(objs))
		return false
	}

	accessor := meta.NewAccessor()
	objKeys := make(map[string]struct{})
	for i := range objs {
		kind, _ := accessor.Kind(objs[i])
		ns, _ := accessor.Namespace(objs[i])
		name, _ := accessor.Name(objs[i])
		itemRv, _ := accessor.ResourceVersion(objs[i])

		if namespaced {
			objKeys[fmt.Sprintf("%s-%s-%s-%s", strings.ToLower(kind), ns, name, itemRv)] = struct{}{}
		} else {
			objKeys[fmt.Sprintf("%s-%s-%s", strings.ToLower(kind), name, itemRv)] = struct{}{}
		}
	}

	if len(objKeys) != len(keys) {
		t.Errorf("expect %d keys, but got %d object keys", len(keys), len(objKeys))
		return false
	}

	for key := range objKeys {
		if _, ok := keys[key]; !ok {
			t.Errorf("got unexpected object with key: %s", key)
			return false
		}
	}

	return true
}

func TestCanCacheFor(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	s := NewStorageWrapper(dStorage)

	type proxyRequest struct {
		userAgent string
		verb      string
		path      string
		header    map[string]string
	}

	testcases := map[string]struct {
		cacheAgents    string
		preRequest     *proxyRequest
		preExpectCache bool
		request        *proxyRequest
		expectCache    bool
	}{
		"no user agent": {
			request: &proxyRequest{
				verb: "GET",
				path: "/api/v1/nodes/mynode",
			},
			expectCache: false,
		},
		"not default user agent": {
			request: &proxyRequest{
				userAgent: "kubelet-test",
				verb:      "GET",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: false,
		},
		"default user agent kubelet": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: true,
		},
		"default user agent flanneld": {
			request: &proxyRequest{
				userAgent: "flanneld",
				verb:      "POST",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: true,
		},
		"default user agent coredns": {
			request: &proxyRequest{
				userAgent: "coredns",
				verb:      "PUT",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: true,
		},
		"default user agent kube-proxy": {
			request: &proxyRequest{
				userAgent: "kube-proxy",
				verb:      "PATCH",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: true,
		},
		"default user agent tunnel-agent": {
			request: &proxyRequest{
				userAgent: projectinfo.GetAgentName(),
				verb:      "HEAD",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: true,
		},
		"with cache header": {
			request: &proxyRequest{
				userAgent: "test1",
				verb:      "GET",
				path:      "/api/v1/nodes/mynode",
				header:    map[string]string{"Edge-Cache": "true"},
			},
			expectCache: true,
		},
		"with cache header false": {
			request: &proxyRequest{
				userAgent: "test2",
				verb:      "GET",
				path:      "/api/v1/nodes/mynode",
				header:    map[string]string{"Edge-Cache": "false"},
			},
			expectCache: false,
		},
		"not resource request": {
			request: &proxyRequest{
				userAgent: "test2",
				verb:      "GET",
				path:      "/healthz",
				header:    map[string]string{"Edge-Cache": "true"},
			},
			expectCache: false,
		},
		"delete request": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "DELETE",
				path:      "/api/v1/nodes/mynode",
			},
			expectCache: false,
		},
		"delete collection request": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "DELETE",
				path:      "/api/v1/namespaces/default/pods",
			},
			expectCache: false,
		},
		"proxy request": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/proxy/namespaces/default/pods/test",
			},
			expectCache: false,
		},
		"get status sub resource request": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods/test/status",
			},
			expectCache: true,
		},
		"get not status sub resource request": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods/test/proxy",
			},
			expectCache: false,
		},
		"list requests with no selectors": {
			preRequest: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods",
			},
			preExpectCache: true,
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods",
			},
			expectCache: true,
		},
		"list requests with label selectors": {
			preRequest: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/kube-system/pods?labelSelector=foo=bar",
			},
			preExpectCache: true,
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/kube-system/pods?labelSelector=foo=bar",
			},
			expectCache: true,
		},
		"list requests with field selectors": {
			preRequest: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/test2/pods?fieldSelector=spec.nodeName=test",
			},
			preExpectCache: true,
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/test2/pods?fieldSelector=spec.nodeName=test",
			},
			expectCache: true,
		},
		"list requests have same path but with different selectors": {
			preRequest: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/test2/secrets?labelSelector=foo=bar1",
			},
			preExpectCache: true,
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/test2/secrets?labelSelector=foo=bar2",
			},
			expectCache: false,
		},
		"list requests get same resouces but with different path": {
			preRequest: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/namespaces/test2/configmaps?labelSelector=foo=bar1",
			},
			preExpectCache: true,
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "GET",
				path:      "/api/v1/configmaps?labelSelector=foo=bar2",
			},
			expectCache: false,
		},
		"cacheAgents *": {
			request: &proxyRequest{
				userAgent: "lc",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods/test/status",
			},
			cacheAgents: "*",
			expectCache: true,
		},
		"cacheAgents *  for old": {
			request: &proxyRequest{
				userAgent: "lc",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods/test/status",
			},
			cacheAgents: "*,xxx",
			expectCache: true,
		},
		"cacheAgents without *": {
			request: &proxyRequest{
				userAgent: "lc",
				verb:      "GET",
				path:      "/api/v1/namespaces/default/pods/test/status",
			},
			cacheAgents: "xxx",
			expectCache: false,
		},
		"do not cache csr": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "POST",
				path:      "/apis/certificates.k8s.io/v1/certificatesigningrequests",
			},
			expectCache: false,
		},
		"do not cache sar": {
			request: &proxyRequest{
				userAgent: "kubelet",
				verb:      "POST",
				path:      "/apis/authorization.k8s.io/v1/subjectaccessreviews",
			},
			expectCache: false,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			stop := make(chan struct{})
			defer close(stop)
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			m := NewCacheManager(s, nil, nil, informerFactory)
			informerFactory.Start(nil)
			cache.WaitForCacheSync(stop, informerFactory.Core().V1().ConfigMaps().Informer().HasSynced)
			if tt.preRequest != nil {
				reqCanCache := checkReqCanCache(m, tt.preRequest.userAgent, tt.preRequest.verb, tt.preRequest.path, tt.preRequest.header, tt.cacheAgents, client)
				if reqCanCache != tt.preExpectCache {
					t.Errorf("Got request pre can cache %v, but expect request pre can cache %v", reqCanCache, tt.preExpectCache)
				}
			}

			if tt.request != nil {
				reqCanCache := checkReqCanCache(m, tt.request.userAgent, tt.request.verb, tt.request.path, tt.request.header, tt.cacheAgents, client)
				if reqCanCache != tt.expectCache {
					t.Errorf("Got request can cache %v, but expect request can cache %v", reqCanCache, tt.expectCache)
				}
			}
		})
	}

	if err = os.RemoveAll(rootDir); err != nil {
		t.Errorf("Got error %v, unable to remove path %s", err, rootDir)
	}
}

func checkReqCanCache(m CacheManager, userAgent, verb, path string, header map[string]string, cacheAgents string, testClient *fake.Clientset) bool {
	req, _ := http.NewRequest(verb, path, nil)
	if len(userAgent) != 0 {
		req.Header.Set("User-Agent", userAgent)
	}

	for k, v := range header {
		req.Header.Set(k, v)
	}

	req.RemoteAddr = "127.0.0.1"
	if cacheAgents != "" {
		_, err := testClient.CoreV1().ConfigMaps(util.YurtHubNamespace).Create(context.Background(), &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.YurthubConfigMapName,
				Namespace: util.YurtHubNamespace,
			},
			Data: map[string]string{
				util.CacheUserAgentsKey: cacheAgents,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return false
		}
		// waiting for create event
		time.Sleep(2 * time.Second)
	}
	var reqCanCache bool
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqCanCache = m.CanCacheFor(req)

	})

	handler = proxyutil.WithListRequestSelector(handler)
	handler = proxyutil.WithCacheHeaderCheck(handler)
	handler = proxyutil.WithRequestClientComponent(handler)
	handler = filters.WithRequestInfo(handler, newTestRequestInfoResolver())
	handler.ServeHTTP(httptest.NewRecorder(), req)

	return reqCanCache
}

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestIsListRequestWithNameFieldSelector(t *testing.T) {
	testcases := map[string]struct {
		Verb   string
		Path   string
		Expect bool
	}{
		"request has metadata.name fieldSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			Expect: true,
		},
		"request has no metadata.name fieldSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=spec.nodeName=test",
			Expect: false,
		},
		"request only has labelSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&labelSelector=foo=bar",
			Expect: false,
		},
		"request has both labelSelector and fieldSelector and fieldSelector has metadata.name": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?fieldSelector=metadata.name=test&labelSelector=foo=bar",
			Expect: true,
		},
		"request has both labelSelector and fieldSelector but fieldSelector has no metadata.name": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?fieldSelector=spec.nodeName=test&labelSelector=foo=bar",
			Expect: false,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			req.RemoteAddr = "127.0.0.1"

			var isMetadataNameFieldSelector bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				isMetadataNameFieldSelector = isListRequestWithNameFieldSelector(req)
			})

			handler = proxyutil.WithListRequestSelector(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isMetadataNameFieldSelector != tc.Expect {
				t.Errorf("failed at case %s, want: %v, got: %v", k, tc.Expect, isMetadataNameFieldSelector)
			}
		})
	}
}

// TODO: in-memory cache unit tests
