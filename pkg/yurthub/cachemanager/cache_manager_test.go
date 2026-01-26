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
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	rootDir = "/tmp/cache-manager"
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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

	testcases := map[string]struct {
		inputObj     runtime.Object
		header       map[string]string
		verb         string
		path         string
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb:             "GET",
			path:             "/api/v1/namespaces/default/pods/mypod1",
			cacheResponseErr: true,
		},
		"cache response for get pod": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods/mypod1",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods/mypod2",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes/mynode1",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes/mynode2",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab1",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs/crontab2",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/samplecontroller.k8s.io/v1/foos/foo1",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/samplecontroller.k8s.io/v1/foos/foo2",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes/test",
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
			inputObj: nil,
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb:             "GET",
			path:             "/api/v1/nodes/test",
			cacheResponseErr: true,
		},
		"cache response for get namespace": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/kube-system",
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
		"cache response for partial object metadata request": {
			inputObj: runtime.Object(&metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "meta.k8s.io/v1",
					Kind:       "PartialObjectMetadata",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nodepools.apps.openyurt.io",
					ResourceVersion: "738",
					UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab900",
				},
			}),
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json;as=PartialObjectMetadata;g=meta.k8s.io;v=v1",
			},
			verb: "GET",
			path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/nodepools.apps.openyurt.io",
			expectResult: struct {
				err  error
				rv   string
				name string
				ns   string
				kind string
			}{
				rv:   "738",
				name: "nodepools.apps.openyurt.io",
				kind: "PartialObjectMetadata",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = "127.0.0.1"

			var cacheErr error
			var info *request.RequestInfo
			var comp string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				info, _ = request.RequestInfoFrom(ctx)
				// get component
				comp, _ = util.TruncatedClientComponentFrom(ctx)

				// inject response content type by request content type
				reqContentType, _ := util.ReqContentTypeFrom(ctx)
				ctx = util.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)

				// build response body
				gvr := schema.GroupVersionResource{
					Group:    info.APIGroup,
					Version:  info.APIVersion,
					Resource: info.Resource,
				}

				convertGVK, ok := util.ConvertGVKFrom(ctx)
				if ok && convertGVK != nil {
					gvr, _ = meta.UnsafeGuessKindToResource(*convertGVK)
					comp = util.AttachConvertGVK(comp, convertGVK)
				}

				s := serializerM.CreateSerializer(reqContentType, gvr.Group, gvr.Version, gvr.Resource)
				encoder, err := s.Encoder(reqContentType, nil)
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
				prc := io.NopCloser(buf)
				cacheErr = yurtCM.CacheResponse(req, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = proxyutil.WithPartialObjectMetadataRequest(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.cacheResponseErr && cacheErr == nil {
				t.Errorf("expect err, but do not get error")
			} else if !tt.cacheResponseErr && cacheErr != nil {
				t.Errorf("expect no err, but got error %v", err)
			} else if tt.cacheResponseErr && cacheErr != nil {
				return
			}

			keyInfo := storage.KeyBuildInfo{
				Component: comp,
				Namespace: info.Namespace,
				Name:      info.Name,
				Resources: info.Resource,
				Group:     info.APIGroup,
				Version:   info.APIVersion,
			}
			key, err := sWrapper.KeyFunc(keyInfo)
			if err != nil {
				t.Errorf("failed to create key, %v", err)
			}
			obj, err := sWrapper.Get(key)
			if err != nil || obj == nil {
				if !errors.Is(tt.expectResult.err, err) {
					t.Errorf("expect get error %v, but got %v", tt.expectResult.err, err)
				}
				t.Logf("get expected err %v for key %s", tt.expectResult.err, keyInfo)
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

				ns, _ := accessor.Namespace(obj)
				if tt.expectResult.ns != ns {
					t.Errorf("Got ns %s, but expect ns %s", ns, tt.expectResult.ns)
				}

				if tt.expectResult.kind != kind {
					t.Errorf("Got kind %s, but expect kind %s", kind, tt.expectResult.kind)
				}
				t.Logf("get key %s successfully", keyInfo)
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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

	testcases := map[string]struct {
		inputObj     []watch.Event
		header       map[string]string
		verb         string
		path         string
		expectResult struct {
			err  bool
			data map[string]struct{}
		}
	}{
		"add pods": {
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Added, Object: mkPod("mypod2", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Deleted, Object: mkPod("mypod1", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "2")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Added, Object: mkPod("mypod3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkPod("mypod1", "6")},
				{Type: watch.Modified, Object: mkPod("mypod1", "4")},
				{Type: watch.Modified, Object: mkPod("mypod1", "2")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Added, Object: mkCronTab("crontab2", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Deleted, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "2")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Added, Object: mkCronTab("crontab3", "6")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Added, Object: mkCronTab("crontab1", "6")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "4")},
				{Type: watch.Modified, Object: mkCronTab("crontab1", "2")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs?watch=true",
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
			inputObj: []watch.Event{
				{Type: watch.Bookmark, Object: mkPod("mypod1", "2")},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"add pods for partial object metadata watch request": {
			inputObj: []watch.Event{
				{Type: watch.Added, Object: runtime.Object(&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod1",
						Namespace:       "default",
						ResourceVersion: "2",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab900",
					},
				})},
				{Type: watch.Added, Object: runtime.Object(&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "mypod2",
						Namespace:       "default",
						ResourceVersion: "3",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab901",
					},
				})},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json;as=PartialObjectMetadata;g=meta.k8s.io;v=v1",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?watch=true",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"partialobjectmetadata-default-mypod1-2": {},
					"partialobjectmetadata-default-mypod2-3": {},
				},
			},
		},
		"add crontabs for partial object metadata watch request": {
			inputObj: []watch.Event{
				{Type: watch.Added, Object: runtime.Object(&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "crontab1",
						ResourceVersion: "2",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab900",
					},
				})},
				{Type: watch.Added, Object: runtime.Object(&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "crontab2",
						ResourceVersion: "3",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab901",
					},
				})},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json;as=PartialObjectMetadata;g=meta.k8s.io;v=v1",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/crontabs?watch=true",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"partialobjectmetadata-crontab1-2": {},
					"partialobjectmetadata-crontab2-3": {},
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = "127.0.0.1"

			var err error
			var info *request.RequestInfo
			var comp string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				info, _ = request.RequestInfoFrom(ctx)
				// get component
				comp, _ = util.TruncatedClientComponentFrom(ctx)

				// inject response content type by request content type
				reqContentType, _ := util.ReqContentTypeFrom(ctx)
				ctx = util.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)

				// build response body
				gvr := schema.GroupVersionResource{
					Group:    info.APIGroup,
					Version:  info.APIVersion,
					Resource: info.Resource,
				}

				convertGVK, ok := util.ConvertGVKFrom(ctx)
				if ok && convertGVK != nil {
					gvr, _ = meta.UnsafeGuessKindToResource(*convertGVK)
					comp = util.AttachConvertGVK(comp, convertGVK)
				}

				s := serializerM.CreateSerializer(reqContentType, gvr.Group, gvr.Version, gvr.Resource)
				pr, pw := io.Pipe()
				go func(pw *io.PipeWriter) {
					//For unregistered GVKs, the normal encoding is used by default and the original GVK information is set
					for i := range tt.inputObj {
						if _, err := s.WatchEncode(pw, &tt.inputObj[i]); err != nil {
							t.Errorf("%d: encode watch unexpected error: %v", i, err)
							continue
						}
						time.Sleep(100 * time.Millisecond)
					}
					pw.Close()
				}(pw)
				rc := io.NopCloser(pr)
				err = yurtCM.CacheResponse(req, rc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = proxyutil.WithPartialObjectMetadataRequest(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err && err == nil {
				t.Errorf("expect err, but do not got err")
			} else if err != nil && err != io.EOF {
				t.Errorf("failed to cache response, %v", err)
			}

			if len(tt.expectResult.data) == 0 {
				return
			}

			keyInfo := storage.KeyBuildInfo{
				Component: comp,
				Namespace: info.Namespace,
				Name:      info.Name,
				Resources: info.Resource,
				Group:     info.APIGroup,
				Version:   info.APIVersion,
			}
			rootKey, err := sWrapper.KeyFunc(keyInfo)
			if err != nil {
				t.Errorf("failed to get key, %v", err)
			}

			objs, err := sWrapper.List(rootKey)
			if err != nil || len(objs) == 0 {
				t.Errorf("failed to get object from storage")
			}

			if !compareObjectsAndKeys(t, objs, tt.expectResult.data) {
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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

	testcases := map[string]struct {
		inputObj     runtime.Object
		header       map[string]string
		verb         string
		path         string
		expectResult struct {
			err  bool
			data map[string]struct{}
		}
	}{
		"list pods": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes",
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
		"list nodes with fieldSelector": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes?fieldSelector=metadata.name=mynode",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/node.k8s.io/v1beta1/runtimeclasses",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list with status": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/node",
		},
		//used to test whether custom resource list can be cached correctly
		"cache response for list crontabs": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/samplecontroller.k8s.io/v1/foos",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/samplecontroller.k8s.io/v1/foos",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{},
			},
		},
		"list namespaces": {
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces",
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
		"cache response for partial object metadata list request": {
			inputObj: runtime.Object(&metav1.PartialObjectMetadataList{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "meta.k8s.io/v1",
					Kind:       "PartialObjectMetadataList",
				},
				ListMeta: metav1.ListMeta{
					ResourceVersion: "738",
				},
				Items: []metav1.PartialObjectMetadata{
					{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "meta.k8s.io/v1",
							Kind:       "PartialObjectMetadata",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:            "nodepools.apps.openyurt.io",
							UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab900",
							ResourceVersion: "738",
						},
					},
					{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "meta.k8s.io/v1",
							Kind:       "PartialObjectMetadata",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:            "yurtappsets.apps.openyurt.io",
							UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab901",
							ResourceVersion: "737",
						},
					},
				},
			}),
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1",
			},
			verb: "GET",
			path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
			expectResult: struct {
				err  bool
				data map[string]struct{}
			}{
				data: map[string]struct{}{
					"partialobjectmetadata-nodepools.apps.openyurt.io-738":   {},
					"partialobjectmetadata-yurtappsets.apps.openyurt.io-737": {},
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = "127.0.0.1"

			var cacheErr error
			var info *request.RequestInfo
			var comp string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				info, _ = request.RequestInfoFrom(ctx)
				// get component
				comp, _ = util.TruncatedClientComponentFrom(ctx)

				// inject response content type by request content type
				reqContentType, _ := util.ReqContentTypeFrom(ctx)
				ctx = util.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)

				// build response body
				gvr := schema.GroupVersionResource{
					Group:    info.APIGroup,
					Version:  info.APIVersion,
					Resource: info.Resource,
				}

				convertGVK, ok := util.ConvertGVKFrom(ctx)
				if ok && convertGVK != nil {
					gvr, _ = meta.UnsafeGuessKindToResource(*convertGVK)
					comp = util.AttachConvertGVK(comp, convertGVK)
				}

				s := serializerM.CreateSerializer(reqContentType, gvr.Group, gvr.Version, gvr.Resource)
				encoder, err := s.Encoder(reqContentType, nil)
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
				prc := io.NopCloser(buf)
				// call cache response
				cacheErr = yurtCM.CacheResponse(req, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = proxyutil.WithPartialObjectMetadataRequest(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.expectResult.err {
				if cacheErr == nil {
					t.Error("Got no error, but expect err")
					return
				}
			} else {
				if err != nil {
					t.Errorf("Expect no error, but got error %v", err)
					return
				}

				keyInfo := storage.KeyBuildInfo{
					Component: comp,
					Namespace: info.Namespace,
					Name:      info.Name,
					Resources: info.Resource,
					Group:     info.APIGroup,
					Version:   info.APIVersion,
				}
				rootKey, err := sWrapper.KeyFunc(keyInfo)
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

				if !compareObjectsAndKeys(t, objs, tt.expectResult.data) {
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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

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
// 	yurtCM := NewCacheManager(fakeClient, sWrapper, serializerM, restRESTMapperMgr, fakeSharedInformerFactory)

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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

	testcases := map[string]struct {
		keyBuildInfo *storage.KeyBuildInfo
		inputObj     []runtime.Object
		header       map[string]string
		verb         string
		path         string
		expectResult struct {
			err      bool
			queryErr error
			rv       string
			data     map[string]struct{}
		}
	}{
		"list with no user agent": {
			header: map[string]string{
				"Accept": "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes",
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
		"list pods of one namespace and no pods of this namespace in cache": {
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/pods/namespaces/default",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/stable.example.com/v1/namespaces/default/crontabs",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/samplecontroller.k8s.io/v1/foos",
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
		"list unregistered resources": {
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/apis/sample.k8s.io/v1/abcs",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/kube-system/configmaps?fieldSelector=metadata.name%3Dkubernetes-services-endpoint",
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
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/namespaces/default/pods?fieldSelector=metadata.name%3Dnginx",
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
		"list crds by partial object metadata request": {
			keyBuildInfo: &storage.KeyBuildInfo{
				Component: "kubelet/partialobjectmetadatas.v1.meta.k8s.io",
				Resources: "customresourcedefinitions",
				Group:     "apiextensions.k8s.io",
				Version:   "v1",
			},
			inputObj: []runtime.Object{
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "nodepools.apps.openyurt.io",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab900",
						ResourceVersion: "738",
					},
				},
				&metav1.PartialObjectMetadata{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "meta.k8s.io/v1",
						Kind:       "PartialObjectMetadata",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "yurtappsets.apps.openyurt.io",
						UID:             "4232ad7f-c347-43a3-b64a-1c4bdfeab901",
						ResourceVersion: "737",
					},
				},
			},
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1",
			},
			verb: "GET",
			path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
			expectResult: struct {
				err      bool
				queryErr error
				rv       string
				data     map[string]struct{}
			}{
				rv: "738",
				data: map[string]struct{}{
					"partialobjectmetadata-nodepools.apps.openyurt.io-738":   {},
					"partialobjectmetadata-yurtappsets.apps.openyurt.io-737": {},
				},
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			for i := range tt.inputObj {
				name, _ := accessor.Name(tt.inputObj[i])
				ns, _ := accessor.Namespace(tt.inputObj[i])
				gvk := tt.inputObj[i].GetObjectKind().GroupVersionKind()
				gvr, _ := meta.UnsafeGuessKindToResource(gvk)
				comp := tt.header["User-Agent"]

				keyBuildInfo := storage.KeyBuildInfo{
					Component: comp,
					Resources: gvr.Resource,
					Group:     gvr.Group,
					Version:   gvr.Version,
					Namespace: ns,
					Name:      name,
				}

				if tt.keyBuildInfo != nil {
					tt.keyBuildInfo.Name = name
					tt.keyBuildInfo.Namespace = ns
					keyBuildInfo = *tt.keyBuildInfo
				}

				key, err := sWrapper.KeyFunc(keyBuildInfo)
				if err != nil {
					t.Errorf("failed to get key, %v", err)
				}
				_ = sWrapper.Create(key, tt.inputObj[i])

				isScheme, t := restRESTMapperMgr.KindFor(gvr)
				if !isScheme && t.Empty() {
					_ = restRESTMapperMgr.UpdateKind(gvk)
				}
			}

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
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
			handler = proxyutil.WithPartialObjectMetadataRequest(handler)
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

				if !compareObjectsAndKeys(t, items, tt.expectResult.data) {
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

func TestInMemoryCacheDeepCopy(t *testing.T) {
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

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	configManager := configuration.NewConfigurationManager("node1", fakeSharedInformerFactory)
	yurtCM := NewCacheManager(sWrapper, serializerM, restRESTMapperMgr, configManager)

	testcases := map[string]struct {
		inputObj     runtime.Object
		header       map[string]string
		verb         string
		path         string
		expectResult struct {
			rv   string
			name string
		}
	}{
		"deep copy node on in-memory cache": {
			inputObj: runtime.Object(&v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mynode1",
					ResourceVersion: "1",
				},
			}),
			header: map[string]string{
				"User-Agent": "kubelet",
				"Accept":     "application/json",
			},
			verb: "GET",
			path: "/api/v1/nodes/mynode1",
			expectResult: struct {
				rv   string
				name string
			}{
				rv:   "1",
				name: "mynode1",
			},
		},
	}

	accessor := meta.NewAccessor()
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			originalNode := tt.inputObj.(*v1.Node)
			originalName := originalNode.Name

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			for key, value := range tt.header {
				req.Header.Set(key, value)
			}
			req.RemoteAddr = "127.0.0.1"

			var cacheErr error
			var info *request.RequestInfo
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				info, _ = request.RequestInfoFrom(ctx)
				_, _ = util.TruncatedClientComponentFrom(ctx)

				reqContentType, _ := util.ReqContentTypeFrom(ctx)
				ctx = util.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)

				gvr := schema.GroupVersionResource{
					Group:    info.APIGroup,
					Version:  info.APIVersion,
					Resource: info.Resource,
				}

				s := serializerM.CreateSerializer(reqContentType, gvr.Group, gvr.Version, gvr.Resource)
				encoder, err := s.Encoder(reqContentType, nil)
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
				prc := io.NopCloser(buf)
				cacheErr = yurtCM.CacheResponse(req, prc, nil)
			})

			handler = proxyutil.WithRequestContentType(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if cacheErr != nil {
				t.Errorf("Got error when cache response, %v", cacheErr)
			}

			req2, _ := http.NewRequest(tt.verb, tt.path, nil)
			for key, value := range tt.header {
				req2.Header.Set(key, value)
			}
			req2.RemoteAddr = "127.0.0.1"

			var obj runtime.Object
			var err error
			var handler2 http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				obj, err = yurtCM.QueryCache(req)
			})

			handler2 = proxyutil.WithRequestClientComponent(handler2)
			handler2 = filters.WithRequestInfo(handler2, resolver)
			handler2.ServeHTTP(httptest.NewRecorder(), req2)

			if err != nil {
				t.Errorf("Got error %v", err)
			}

			if obj == nil {
				t.Errorf("Got nil obj, but expect obj")
				return
			}

			retrievedNode, ok := obj.(*v1.Node)
			if !ok {
				t.Errorf("Got obj type %T, but expect *v1.Node", obj)
				return
			}

			name, _ := accessor.Name(retrievedNode)
			rv, _ := accessor.ResourceVersion(retrievedNode)

			if tt.expectResult.name != name {
				t.Errorf("Got name %s, but expect name %s", name, tt.expectResult.name)
			}

			if tt.expectResult.rv != rv {
				t.Errorf("Got rv %s, but expect rv %s", rv, tt.expectResult.rv)
			}

			originalNode.Name = "modified-node"
			retrievedNameAfterModify, _ := accessor.Name(retrievedNode)
			if retrievedNameAfterModify == "modified-node" {
				t.Errorf("Got cached name %s after modify original, but expect %s", retrievedNameAfterModify, originalName)
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

func compareObjectsAndKeys(t *testing.T, objs []runtime.Object, keys map[string]struct{}) bool {
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

		if len(ns) != 0 {
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
		"not resource request": {
			request: &proxyRequest{
				userAgent: "test2",
				verb:      "GET",
				path:      "/healthz",
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
		"list requests get same resources but with different path": {
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
		"default user agent kubelet with partialobjectmetadata info": {
			request: &proxyRequest{
				userAgent: "kubelet/v1.0",
				verb:      "GET",
				path:      "/api/v1/nodes/mynode",
				header:    map[string]string{"Accept": "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1"},
			},
			expectCache: true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			stop := make(chan struct{})
			defer close(stop)
			client := fake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)
			configManager := configuration.NewConfigurationManager("node1", informerFactory)
			m := NewCacheManager(s, nil, nil, configManager)
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
	handler = proxyutil.WithRequestClientComponent(handler)
	handler = proxyutil.WithPartialObjectMetadataRequest(handler)
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
