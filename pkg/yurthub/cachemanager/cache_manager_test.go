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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	v1 "k8s.io/api/core/v1"
	nodev1beta1 "k8s.io/api/node/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	runtimejson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/client-go/kubernetes/scheme"
	restclientwatch "k8s.io/client-go/rest/watch"
)

func TestCacheResponse(t *testing.T) {
	storage := NewFakeStorageWrapper()
	serializerM := serializer.NewSerializerManager()
	yurtCM := &cacheManager{
		storage:           storage,
		serializerManager: serializerM,
		cacheAgents:       make(map[string]bool),
	}

	type expectData struct {
		err  bool
		rv   string
		name string
		ns   string
		kind string
	}
	tests := []struct {
		desc         string
		group        string
		version      string
		key          string
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		resource     string
		namespaced   bool
		expectResult expectData
	}{
		{
			desc:    "cache response for get pod",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default/mypod1",
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
			resource:   "pods",
			namespaced: true,
			expectResult: expectData{
				rv:   "1",
				name: "mypod1",
				ns:   "default",
				kind: "Pod",
			},
		},
		{
			desc:    "cache response for get pod2",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default/mypod2",
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
			}),
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod2",
			resource:   "pods",
			namespaced: true,
			expectResult: expectData{
				rv:   "3",
				name: "mypod2",
				ns:   "default",
				kind: "Pod",
			},
		},
		{
			desc:    "cache response for get node",
			group:   "",
			version: "v1",
			key:     "kubelet/nodes/mynode1",
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
			expectResult: expectData{
				rv:   "4",
				name: "mynode1",
				kind: "Node",
			},
		},
		{
			desc:    "cache response for get node2",
			group:   "",
			version: "v1",
			key:     "kubelet/nodes/mynode2",
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
			expectResult: expectData{
				rv:   "6",
				name: "mynode2",
				kind: "Node",
			},
		},

		//used to test whether custom resources can be cached correctly
		{
			desc:    "cache response for get crontab",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default/crontab1",
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
			expectResult: expectData{
				rv:   "1",
				name: "crontab1",
				ns:   "default",
				kind: "CronTab",
			},
		},
		{
			desc:    "cache response for get crontab2",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default/crontab2",
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
			expectResult: expectData{
				rv:   "3",
				name: "crontab2",
				ns:   "default",
				kind: "CronTab",
			},
		},
		{
			desc:    "cache response for get foo without namespace",
			group:   "samplecontroller.k8s.io",
			version: "v1",
			key:     "kubelet/foos/foo1",
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
			expectResult: expectData{
				rv:   "3",
				name: "foo1",
				kind: "Foo",
			},
		},
		{
			desc:    "cache response for get foo2 without namespace",
			group:   "samplecontroller.k8s.io",
			version: "v1",
			key:     "kubelet/foos/foo2",
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
			expectResult: expectData{
				rv:   "5",
				name: "foo2",
				kind: "Foo",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			encoder, err := serializerM.CreateSerializers(tt.accept, tt.group, tt.version, tt.resource)
			if err != nil {
				t.Fatalf("could not create serializer, %v", err)
			}

			buf := bytes.NewBuffer([]byte{})
			err = encoder.Encoder.Encode(tt.inputObj, buf)
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
				prc := ioutil.NopCloser(buf)
				err = yurtCM.CacheResponse(ctx, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
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

				obj, err := storage.Get(tt.key)
				if err != nil || obj == nil {
					t.Errorf("failed to get object from storage")
				}

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
			}
		})
	}
}

func getEncoder() runtime.Encoder {
	jsonSerializer := runtimejson.NewSerializer(runtimejson.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, false)
	directCodecFactory := runtimeserializer.WithoutConversionCodecFactory{
		CodecFactory: scheme.Codecs,
	}
	return directCodecFactory.EncoderForVersion(jsonSerializer, v1.SchemeGroupVersion)
}

func resetStorage(s StorageWrapper, key string) {
	keys, _ := s.ListKeys(key)
	for i := range keys {
		s.Delete(keys[i])
	}
}

func TestCacheResponseForWatch(t *testing.T) {
	mkPod := func(id string, rv string) *v1.Pod {
		return &v1.Pod{
			TypeMeta:   metav1.TypeMeta{APIVersion: "", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: "default", ResourceVersion: rv},
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

	storage := NewFakeStorageWrapper()
	serializerM := serializer.NewSerializerManager()
	yurtCM := &cacheManager{
		storage:           storage,
		serializerManager: serializerM,
		cacheAgents:       make(map[string]bool),
	}

	type expectData struct {
		err  bool
		data map[string]struct{}
	}
	tests := []struct {
		desc         string
		group        string
		version      string
		key          string
		inputObj     []watch.Event
		userAgent    string
		accept       string
		verb         string
		path         string
		namespaced   bool
		expectResult expectData
	}{
		{
			desc:    "cache response for watch add pods",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Added, Object: mkPod("mypod2", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"pod-default-mypod1-2": {},
					"pod-default-mypod2-4": {},
					"pod-default-mypod3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch add and delete pods",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default",
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
			expectResult: expectData{
				data: map[string]struct{}{
					"pod-default-mypod3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch add and update pods",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"pod-default-mypod1-4": {},
					"pod-default-mypod3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch not update pods",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "6")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Modified, Object: mkPod("mypod1", "2")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"pod-default-mypod1-6": {},
				},
			},
		},

		//used to test whether custom resource's watch-events can be cached correctly
		{
			desc:    "cache response for watch add crontabs",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Added, Object: mkCronTab("crontab2", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"crontab-default-crontab1-2": {},
					"crontab-default-crontab2-4": {},
					"crontab-default-crontab3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch add and delete crontabs",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Deleted, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"crontab-default-crontab3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch add and update crontabs",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"crontab-default-crontab1-4": {},
					"crontab-default-crontab3-6": {},
				},
			},
		},
		{
			desc:    "cache response for watch not update crontabs",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default",
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "6")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "2")},
			},
			userAgent:  "kubelet",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
			namespaced: true,
			expectResult: expectData{
				data: map[string]struct{}{
					"crontab-default-crontab1-6": {},
				},
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			r, w := io.Pipe()
			go func(w *io.PipeWriter) {
				//For unregistered GVKs, the normal encoding is used by default and the original GVK information is set
				encoder := restclientwatch.NewEncoder(streaming.NewEncoder(w, getEncoder()), getEncoder())

				for i := range tt.inputObj {
					if err := encoder.Encode(&tt.inputObj[i]); err != nil {
						t.Errorf("%d: unexpected error: %v", i, err)
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
			rc := ioutil.NopCloser(r)
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				ctx = util.WithRespContentType(ctx, tt.accept)
				err = yurtCM.CacheResponse(ctx, rc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err {
				if err == nil {
					t.Errorf("Got no error, but expect err")
				}
			} else {
				if err != nil && err != io.EOF {
					t.Errorf("Got error %v", err)
				}

				objs, err := storage.List(tt.key)
				if err != nil || len(objs) == 0 {
					t.Errorf("failed to get object from storage")
				}

				if len(objs) != len(tt.expectResult.data) {
					t.Errorf("Got %d objects, but expect %d objects", len(objs), len(tt.expectResult.data))
				}

				for _, obj := range objs {
					name, _ := accessor.Name(obj)
					ns, _ := accessor.Namespace(obj)
					rv, _ := accessor.ResourceVersion(obj)
					kind, _ := accessor.Kind(obj)

					var objKey string
					if tt.namespaced {
						objKey = fmt.Sprintf("%s-%s-%s-%s", strings.ToLower(kind), ns, name, rv)
					} else {
						objKey = fmt.Sprintf("%s-%s-%s", strings.ToLower(kind), name, rv)
					}

					if _, ok := tt.expectResult.data[objKey]; !ok {
						t.Errorf("Got %s %s/%s with rv %s", kind, ns, name, rv)
					}
				}
				resetStorage(storage, tt.key)
			}
		})
	}
}

func TestCacheResponseForList(t *testing.T) {
	storage := NewFakeStorageWrapper()
	serializerM := serializer.NewSerializerManager()
	yurtCM := &cacheManager{
		storage:           storage,
		serializerManager: serializerM,
		cacheAgents:       make(map[string]bool),
	}

	type expectData struct {
		err  bool
		data map[string]struct{}
	}
	tests := []struct {
		desc         string
		group        string
		version      string
		key          string
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		resource     string
		namespaced   bool
		expectResult expectData
	}{
		{
			desc:    "cache response for list pods",
			group:   "",
			version: "v1",
			key:     "kubelet/pods/default",
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
			expectResult: expectData{
				data: map[string]struct{}{
					"pod-default-mypod1-1": {},
					"pod-default-mypod2-3": {},
					"pod-default-mypod3-5": {},
				},
			},
		},
		{
			desc:    "cache response for list nodes",
			group:   "",
			version: "v1",
			key:     "kubelet/nodes",
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
			expectResult: expectData{
				data: map[string]struct{}{
					"node-mynode1-6":  {},
					"node-mynode2-8":  {},
					"node-mynode3-10": {},
					"node-mynode4-12": {},
				},
			},
		},
		{
			desc:    "cache response for list nodes with fieldselector",
			group:   "",
			version: "v1",
			key:     "kubelet/nodes",
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
			namespaced: false,
			expectResult: expectData{
				data: map[string]struct{}{
					"node-mynode-12": {},
				},
			},
		},
		{
			desc:    "cache response for list runtimeclasses with no objects",
			group:   "node.k8s.io",
			version: "v1beta1",
			key:     "kubelet/runtimeclass",
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
			namespaced: false,
			expectResult: expectData{
				data: map[string]struct{}{},
			},
		},

		//used to test whether custom resource list can be cached correctly
		{
			desc:    "cache response for list crontabs",
			group:   "stable.example.com",
			version: "v1",
			key:     "kubelet/crontabs/default",
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
			expectResult: expectData{
				data: map[string]struct{}{
					"crontab-default-crontab1-1": {},
					"crontab-default-crontab2-2": {},
				},
			},
		},
		{
			desc:    "cache response for list foos without namespace",
			group:   "samplecontroller.k8s.io",
			version: "v1",
			key:     "kubelet/foos",
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
			expectResult: expectData{
				data: map[string]struct{}{
					"foo-foo1-1": {},
					"foo-foo2-2": {},
				},
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			encoder, err := serializerM.CreateSerializers(tt.accept, tt.group, tt.version, tt.resource)
			if err != nil {
				t.Fatalf("could not create serializer, %v", err)
			}

			buf := bytes.NewBuffer([]byte{})
			err = encoder.Encoder.Encode(tt.inputObj, buf)
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
				prc := ioutil.NopCloser(buf)
				err = yurtCM.CacheResponse(ctx, prc, nil)
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

				objs, err := storage.List(tt.key)
				if err != nil {
					t.Errorf("failed to list objects from storage, %v", err)
				}

				if len(objs) != len(tt.expectResult.data) {
					t.Errorf("Got %d objects, but expect %d objects", len(objs), len(tt.expectResult.data))
				}

				for _, obj := range objs {
					name, _ := accessor.Name(obj)
					ns, _ := accessor.Namespace(obj)
					rv, _ := accessor.ResourceVersion(obj)
					kind, _ := accessor.Kind(obj)

					var objKey string
					if tt.namespaced {
						objKey = fmt.Sprintf("%s-%s-%s-%s", strings.ToLower(kind), ns, name, rv)
					} else {
						objKey = fmt.Sprintf("%s-%s-%s", strings.ToLower(kind), name, rv)
					}

					if _, ok := tt.expectResult.data[objKey]; !ok {
						t.Errorf("Got %s %s/%s with rv %s", kind, ns, name, rv)
					}
				}
			}
			resetStorage(storage, tt.key)
		})
	}
}

func TestQueryCacheForGet(t *testing.T) {
	storage := NewFakeStorageWrapper()
	serializerM := serializer.NewSerializerManager()
	yurtCM := &cacheManager{
		storage:           storage,
		serializerManager: serializerM,
		cacheAgents:       make(map[string]bool),
	}

	type expectData struct {
		err  bool
		rv   string
		name string
		ns   string
		kind string
	}
	tests := []struct {
		desc         string
		key          string
		inputObj     runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		namespaced   bool
		expectResult expectData
	}{
		{
			desc:       "no client",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods/mypod1",
			namespaced: true,
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc:       "not resource request",
			accept:     "application/json",
			verb:       "GET",
			path:       "/healthz",
			namespaced: true,
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc: "query post pod",
			key:  "kubelet/pods/default/mypod1",
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
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
			path:       "/api/v1/namespaces/default/pods/mypod1",
			namespaced: true,
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc: "query get pod",
			key:  "kubelet/pods/default/mypod1",
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
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
			expectResult: expectData{
				rv:   "1",
				name: "mypod1",
				ns:   "default",
				kind: "Pod",
			},
		},
		{
			desc: "query update pod",
			key:  "kubelet/pods/default/mypod2",
			inputObj: runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
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
			expectResult: expectData{
				rv:   "2",
				name: "mypod2",
				ns:   "default",
				kind: "Pod",
			},
		},
		{
			desc: "query update node",
			key:  "kubelet/nodes/mynode1",
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind: "Node",
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
			expectResult: expectData{
				rv:   "3",
				name: "mynode1",
				kind: "Node",
			},
		},
		{
			desc: "query patch node",
			key:  "kubelet/nodes/mynode2",
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind: "Node",
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
			expectResult: expectData{
				rv:   "4",
				name: "mynode2",
				kind: "Node",
			},
		},

		//used to test whether the query local Custom Resource request can be handled correctly
		{
			desc:       "no client",
			accept:     "application/json",
			verb:       "GET",
			path:       "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
			namespaced: true,
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc: "query post crontab",
			key:  "kubelet/crontabs/default/crontab1",
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
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc: "query get crontab",
			key:  "kubelet/crontabs/default/crontab1",
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
			expectResult: expectData{
				rv:   "1",
				name: "crontab1",
				ns:   "default",
				kind: "CronTab",
			},
		},
		{
			desc: "query update crontab",
			key:  "kubelet/crontabs/default/crontab2",
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
			expectResult: expectData{
				rv:   "2",
				name: "crontab2",
				ns:   "default",
				kind: "CronTab",
			},
		},
		{
			desc: "query patch crontab",
			key:  "kubelet/crontabs/default/crontab3",
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
			expectResult: expectData{
				rv:   "4",
				name: "crontab3",
				ns:   "default",
				kind: "CronTab",
			},
		},
		{
			desc: "query post foo",
			key:  "kubelet/foos/foo1",
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
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc: "query get foo",
			key:  "kubelet/foos/foo1",
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
			expectResult: expectData{
				rv:   "1",
				name: "foo1",
				kind: "Foo",
			},
		},
		{
			desc: "query update foo",
			key:  "kubelet/foos/foo2",
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
			expectResult: expectData{
				rv:   "2",
				name: "foo2",
				kind: "Foo",
			},
		},
		{
			desc: "query patch foo",
			key:  "kubelet/foos/foo3",
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
			expectResult: expectData{
				rv:   "4",
				name: "foo3",
				kind: "Foo",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_ = storage.Create(tt.key, tt.inputObj)
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.accept) != 0 {
				req.Header.Set("Accept", tt.accept)
			}

			req.RemoteAddr = "127.0.0.1"

			var name, ns, rv, kind string
			var err error
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
		})
	}
}

func TestQueryCacheForList(t *testing.T) {
	storage := NewFakeStorageWrapper()
	serializerM := serializer.NewSerializerManager()
	yurtCM := &cacheManager{
		storage:           storage,
		serializerManager: serializerM,
		cacheAgents:       make(map[string]bool),
	}

	type expectData struct {
		err  bool
		rv   string
		data map[string]struct{}
	}
	tests := []struct {
		desc         string
		keyPrefix    string
		inputObj     []runtime.Object
		userAgent    string
		accept       string
		verb         string
		path         string
		namespaced   bool
		expectResult expectData
	}{
		{
			desc:       "no user agent",
			accept:     "application/json",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/pods",
			namespaced: true,
			expectResult: expectData{
				err: true,
			},
		},
		{
			desc:      "query list pods",
			keyPrefix: "kubelet/pods/default",
			inputObj: []runtime.Object{
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind: "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod1",
						Namespace:       "default",
						ResourceVersion: "1",
					},
				},
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind: "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod2",
						Namespace:       "default",
						ResourceVersion: "2",
					},
				},
				&v1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind: "Pod",
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
			expectResult: expectData{
				rv: "5",
				data: map[string]struct{}{
					"pod-default-mypod1-1": {},
					"pod-default-mypod2-2": {},
					"pod-default-mypod3-5": {},
				},
			},
		},
		{
			desc:      "query list nodes",
			keyPrefix: "kubelet/nodes",
			inputObj: []runtime.Object{
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						Kind: "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode1",
						ResourceVersion: "6",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						Kind: "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode2",
						ResourceVersion: "8",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						Kind: "Node",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mynode3",
						ResourceVersion: "10",
					},
				},
				&v1.Node{
					TypeMeta: metav1.TypeMeta{
						Kind: "Node",
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
			expectResult: expectData{
				rv: "12",
				data: map[string]struct{}{
					"node-mynode1-6":  {},
					"node-mynode2-8":  {},
					"node-mynode3-10": {},
					"node-mynode4-12": {},
				},
			},
		},

		//used to test whether the query local Custom Resource list request can be handled correctly
		{
			desc:      "query list crontabs",
			keyPrefix: "kubelet/crontabs/default",
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
			expectResult: expectData{
				rv: "5",
				data: map[string]struct{}{
					"crontab-default-crontab1-1": {},
					"crontab-default-crontab2-2": {},
					"crontab-default-crontab3-5": {},
				},
			},
		},
		{
			desc:      "query list foos",
			keyPrefix: "kubelet/foos",
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
			expectResult: expectData{
				rv: "5",
				data: map[string]struct{}{
					"foo-foo1-1": {},
					"foo-foo2-2": {},
					"foo-foo3-5": {},
				},
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			for i := range tt.inputObj {
				v, _ := accessor.Name(tt.inputObj[i])
				key := filepath.Join(tt.keyPrefix, v)
				_ = storage.Create(key, tt.inputObj[i])
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
					listMetaInterface, _ := meta.ListAccessor(list)
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
			} else {
				if err != nil {
					t.Errorf("Got error %v", err)
				}

				if tt.expectResult.rv != rv {
					t.Errorf("Got rv %s, but expect rv %s", rv, tt.expectResult.rv)
				}

				if len(items) != len(tt.expectResult.data) {
					t.Errorf("Got %d objects, but expect %d objects", len(items), len(tt.expectResult.data))
				}

				for i := range items {
					kind, _ := accessor.Kind(items[i])
					ns, _ := accessor.Namespace(items[i])
					name, _ := accessor.Name(items[i])
					itemRv, _ := accessor.ResourceVersion(items[i])

					var itemKey string
					if tt.namespaced {
						itemKey = fmt.Sprintf("%s-%s-%s-%s", strings.ToLower(kind), ns, name, itemRv)
					} else {
						itemKey = fmt.Sprintf("%s-%s-%s", strings.ToLower(kind), name, itemRv)
					}

					if expectKey, ok := tt.expectResult.data[itemKey]; !ok {
						t.Errorf("Got item key %s, but expect key %s", itemKey, expectKey)
					}
				}

			}
		})
	}
}

func TestCanCacheFor(t *testing.T) {
	s := NewFakeStorageWrapper()
	m, _ := NewCacheManager(s, nil)

	tests := []struct {
		desc        string
		userAgent   string
		verb        string
		path        string
		header      map[string]string
		expectCache bool
	}{
		{
			desc:        "no user agent",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "not default user agent",
			userAgent:   "kubelet-test",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "default user agent kubelet",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent flanneld",
			userAgent:   "flanneld",
			verb:        "POST",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent coredns",
			userAgent:   "coredns",
			verb:        "PUT",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent kube-proxy",
			userAgent:   "kube-proxy",
			verb:        "PATCH",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent edge-tunnel-agent",
			userAgent:   "edge-tunnel-agent",
			verb:        "HEAD",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "with cache header",
			userAgent:   "test1",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			header:      map[string]string{"Edge-Cache": "true"},
			expectCache: true,
		},
		{
			desc:        "with cache header false",
			userAgent:   "test2",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			header:      map[string]string{"Edge-Cache": "false"},
			expectCache: false,
		},
		{
			desc:        "not resource request",
			userAgent:   "test2",
			verb:        "GET",
			path:        "/healthz",
			header:      map[string]string{"Edge-Cache": "true"},
			expectCache: false,
		},
		{
			desc:        "delete request",
			userAgent:   "kubelet",
			verb:        "DELETE",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "delete collection request",
			userAgent:   "kubelet",
			verb:        "DELETE",
			path:        "/api/v1/namespaces/default/pods",
			expectCache: false,
		},
		{
			desc:        "proxy request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/proxy/namespaces/default/pods/test",
			expectCache: false,
		},
		{
			desc:        "get status sub resource request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/namespaces/default/pods/test/status",
			expectCache: true,
		},
		{
			desc:        "get not status sub resource request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/namespaces/default/pods/test/proxy",
			expectCache: false,
		},
	}

	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.header) != 0 {
				for k, v := range tt.header {
					req.Header.Set(k, v)
				}
			}

			req.RemoteAddr = "127.0.0.1"

			var reqCanCache bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				reqCanCache = m.CanCacheFor(req)

			})

			handler = proxyutil.WithCacheHeaderCheck(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if reqCanCache != tt.expectCache {
				t.Errorf("Got request can cache %v, but expect request can cache %v", reqCanCache, tt.expectCache)
			}
		})
	}
}
