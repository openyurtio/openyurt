/*
Copyright 2022 The OpenYurt Authors.

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

package responsefilter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type nopObjectHandler struct {
	name string
}

func (noh *nopObjectHandler) Name() string {
	return noh.name
}

func (noh *nopObjectHandler) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{}
}

func (noh *nopObjectHandler) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	return obj
}

func TestFilterReadCloser_Read_List(t *testing.T) {
	resolver := newTestRequestInfoResolver()
	sm := serializer.NewSerializerManager()
	handler := &nopObjectHandler{}
	stopCh := make(chan struct{})

	testcases := map[string]struct {
		path      string
		listObj   runtime.Object
		stepSize  int
		expectObj runtime.Object
	}{
		"read list response in one time": {
			path: "/api/v1/services",
			listObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
			stepSize: 32 * 1024,
			expectObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
		},
		"read list response in multiple times": {
			path: "/api/v1/services",
			listObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
			stepSize: 8,
			expectObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}

			req.RemoteAddr = "127.0.0.1"
			req.Header.Set("Accept", "application/json")

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				reqContentType, _ := hubutil.ReqContentTypeFrom(ctx)
				ctx = hubutil.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)
				info, _ := apirequest.RequestInfoFrom(ctx)
				s := createSerializer(reqContentType, info, sm)

				listBytes, _ := s.Encode(tt.listObj)
				buf := bytes.NewBuffer(listBytes)
				rc := io.NopCloser(buf)

				size, newRc, err := newFilterReadCloser(req, sm, rc, handler, "foo", stopCh)
				if err != nil {
					t.Errorf("failed new filter readcloser, %v", err)
				}

				var resBuf bytes.Buffer
				for {
					b := make([]byte, tt.stepSize)
					n, err := newRc.Read(b)
					if err != nil && err != io.EOF {
						t.Errorf("failed to read response %v", err)
					} else if err == io.EOF {
						break
					}

					resBuf.Write(b[:n])
				}

				if size != 0 && size != resBuf.Len() {
					t.Errorf("expect %d bytes, but got %d bytes", size, resBuf.Len())
				}

				readObj, _ := s.Decode(resBuf.Bytes())
				if !reflect.DeepEqual(tt.expectObj, readObj) {
					t.Errorf("expect object \n%#+v\n, but got \n%#+v\n", tt.expectObj, readObj)
				}
				newRc.Close()
			})

			handler = util.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}

func TestFilterReadCloser_Read_Watch(t *testing.T) {
	resolver := newTestRequestInfoResolver()
	sm := serializer.NewSerializerManager()
	handler := &nopObjectHandler{}
	stopCh := make(chan struct{})

	testcases := map[string]struct {
		path        string
		eventType   watch.EventType
		watchObject runtime.Object
		stepSize    int
		expectObj   runtime.Object
	}{
		"read watch response in one time": {
			path:      "/api/v1/services?watch=true",
			eventType: watch.Added,
			watchObject: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			stepSize: 32 * 1024,
			expectObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
		},
		"read watch response in multiple times": {
			path:      "/api/v1/services?watch=true",
			eventType: watch.Added,
			watchObject: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			stepSize: 32,
			expectObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}

			req.RemoteAddr = "127.0.0.1"
			req.Header.Set("Accept", "application/json")

			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				reqContentType, _ := hubutil.ReqContentTypeFrom(ctx)
				ctx = hubutil.WithRespContentType(ctx, reqContentType)
				req = req.WithContext(ctx)
				info, _ := apirequest.RequestInfoFrom(ctx)
				s := createSerializer(reqContentType, info, sm)

				var buf bytes.Buffer
				event := &watch.Event{
					Type:   tt.eventType,
					Object: tt.watchObject,
				}
				initSize, err := s.WatchEncode(&buf, event)
				if err != nil {
					t.Errorf("failed to prepare watch data, %v", err)
				}
				rc := io.NopCloser(&buf)

				_, newRc, err := newFilterReadCloser(req, sm, rc, handler, "foo", stopCh)
				if err != nil {
					t.Errorf("failed new filter readcloser, %v", err)
				}

				var resBuf bytes.Buffer
				for {
					b := make([]byte, tt.stepSize)
					n, err := newRc.Read(b)
					if err != nil && err != io.EOF {
						t.Errorf("failed to read response %v", err)
					} else if err == io.EOF {
						break
					}

					resBuf.Write(b[:n])
				}

				if initSize != resBuf.Len() {
					t.Errorf("expect %d bytes, but got %d bytes", initSize, resBuf.Len())
				}

				resDecoder, _ := s.WatchDecoder(io.NopCloser(&resBuf))
				eType, resObj, _ := resDecoder.Decode()
				if eType != tt.eventType {
					t.Errorf("expect event type %s, but got %s", tt.eventType, eType)
				}

				if !reflect.DeepEqual(tt.expectObj, resObj) {
					t.Errorf("expect object \n%#+v\n, but got \n%#+v\n", tt.expectObj, resObj)
				}
				newRc.Close()
			})

			handler = util.WithRequestContentType(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}

func newTestRequestInfoResolver() *apirequest.RequestInfoFactory {
	return &apirequest.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestResponseFilterForListRequest(t *testing.T) {
	discardCloudSvcFilter, _ := discardcloudservice.NewDiscardCloudServiceFilter()
	nodePortIsolationFilter, _ := nodeportisolation.NewNodePortIsolationFilter()
	if wants, ok := nodePortIsolationFilter.(initializer.WantsNodePoolName); ok {
		if err := wants.SetNodePoolName("hangzhou"); err != nil {
			t.Errorf("cloudn't set pool name, %v", err)
			return
		}
	}
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		objectFilters []filter.ObjectFilter
		group         string
		version       string
		resource      string
		userAgent     string
		verb          string
		path          string
		accept        string
		inputObj      runtime.Object
		names         sets.String
		expectedObj   runtime.Object
	}{
		"verify discard cloud service filter": {
			objectFilters: []filter.ObjectFilter{discardCloudSvcFilter},
			group:         "",
			version:       "v1",
			resource:      "services",
			userAgent:     "kube-proxy",
			verb:          "GET",
			path:          "/api/v1/services",
			accept:        "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								discardcloudservice.DiscardServiceAnnotation: "true",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			names: sets.NewString("discardcloudservice"),
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerManager.CreateSerializer(tc.accept, tc.group, tc.version, tc.resource)
			encoder, err := s.Encoder(tc.accept, nil)
			if err != nil {
				t.Fatalf("could not create encoder, %v", err)
			}

			buf := bytes.NewBuffer([]byte{})
			err = encoder.Encode(tc.inputObj, buf)
			if err != nil {
				t.Fatalf("could not encode input object, %v", err)
			}

			req, err := http.NewRequest(tc.verb, tc.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tc.userAgent) != 0 {
				req.Header.Set("User-Agent", tc.userAgent)
			}

			var newReadCloser io.ReadCloser
			var filterErr error
			var responseFilter filter.ResponseFilter
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				responseFilter = CreateResponseFilter(tc.objectFilters, serializerManager)
				ctx := req.Context()
				ctx = hubutil.WithRespContentType(ctx, tc.accept)
				req = req.WithContext(ctx)
				rc := io.NopCloser(buf)
				_, newReadCloser, filterErr = responseFilter.Filter(req, rc, nil)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if filterErr != nil {
				t.Errorf("get filter err, %v", err)
				return
			}

			names := strings.Split(responseFilter.Name(), ",")
			if !tc.names.Equal(sets.NewString(names...)) {
				t.Errorf("expect filter names %v, but got %v", tc.names.List(), names)
			}

			newBuf, err := io.ReadAll(newReadCloser)
			if err != nil {
				t.Errorf("couldn't read all from new ReadCloser, %v", err)
				return
			}
			newObj, err := s.Decode(newBuf)
			if err != nil {
				t.Errorf("couldn't decode new buf, %v", err)
				return
			}

			if !reflect.DeepEqual(newObj, tc.expectedObj) {
				t.Errorf("expect obj \n%#+v\n, but got \n%#+v\n", tc.expectedObj, newObj)
			}
		})
	}
}

func TestResponseFilterForWatchRequest(t *testing.T) {
	discardCloudSvcFilter, _ := discardcloudservice.NewDiscardCloudServiceFilter()
	nodePortIsolationFilter, _ := nodeportisolation.NewNodePortIsolationFilter()
	if wants, ok := nodePortIsolationFilter.(initializer.WantsNodePoolName); ok {
		if err := wants.SetNodePoolName("hangzhou"); err != nil {
			t.Errorf("cloudn't set pool name, %v", err)
			return
		}
	}
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		objectFilters []filter.ObjectFilter
		group         string
		version       string
		resource      string
		userAgent     string
		verb          string
		path          string
		accept        string
		eventType     watch.EventType
		inputObj      runtime.Object
		names         sets.String
		expectedObj   runtime.Object
	}{
		"verify discardcloudservice filter": {
			objectFilters: []filter.ObjectFilter{discardCloudSvcFilter},
			group:         "",
			version:       "v1",
			resource:      "services",
			userAgent:     "kube-proxy",
			verb:          "GET",
			path:          "/api/v1/services?watch=true",
			accept:        "application/json",
			eventType:     watch.Added,
			inputObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						discardcloudservice.DiscardServiceAnnotation: "true",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			names:       sets.NewString("discardcloudservice"),
			expectedObj: nil,
		},
		"verify discardcloudservice and nodeportisolation filter with nil response": {
			objectFilters: []filter.ObjectFilter{discardCloudSvcFilter, nodePortIsolationFilter},
			group:         "",
			version:       "v1",
			resource:      "services",
			userAgent:     "kube-proxy",
			verb:          "GET",
			path:          "/api/v1/services?watch=true",
			accept:        "application/json",
			eventType:     watch.Added,
			inputObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						discardcloudservice.DiscardServiceAnnotation: "true",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			names:       sets.NewString("discardcloudservice", "nodeportisolation"),
			expectedObj: nil,
		},
		"verify discardcloudservice and nodeportisolation filter normally": {
			objectFilters: []filter.ObjectFilter{discardCloudSvcFilter, nodePortIsolationFilter},
			group:         "",
			version:       "v1",
			resource:      "services",
			userAgent:     "kube-proxy",
			verb:          "GET",
			path:          "/api/v1/services?watch=true",
			accept:        "application/json",
			eventType:     watch.Added,
			inputObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			names: sets.NewString("discardcloudservice", "nodeportisolation"),
			expectedObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			s := serializerManager.CreateSerializer(tc.accept, tc.group, tc.version, tc.resource)
			buf := bytes.NewBuffer([]byte{})
			event := &watch.Event{
				Type:   tc.eventType,
				Object: tc.inputObj,
			}

			_, err := s.WatchEncode(buf, event)
			if err != nil {
				t.Fatalf("could not watch encode input object, %v", err)
			}

			req, err := http.NewRequest(tc.verb, tc.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tc.userAgent) != 0 {
				req.Header.Set("User-Agent", tc.userAgent)
			}

			var newReadCloser io.ReadCloser
			var filterErr error
			var responseFilter filter.ResponseFilter
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				responseFilter = CreateResponseFilter(tc.objectFilters, serializerManager)
				ctx := req.Context()
				ctx = hubutil.WithRespContentType(ctx, tc.accept)
				req = req.WithContext(ctx)
				rc := io.NopCloser(buf)
				_, newReadCloser, filterErr = responseFilter.Filter(req, rc, nil)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if filterErr != nil {
				t.Errorf("get filter err, %v", err)
				return
			}

			names := strings.Split(responseFilter.Name(), ",")
			if !tc.names.Equal(sets.NewString(names...)) {
				t.Errorf("expect filter names %v, but got %v", tc.names.List(), names)
				return
			}

			resDecoder, _ := s.WatchDecoder(newReadCloser)

			for {
				eType, resObj, err := resDecoder.Decode()
				if err != nil {
					klog.Errorf("decode response error, %v", err)
					break
				}

				if tc.expectedObj == nil && resObj == nil {
					continue
				}

				if eType != tc.eventType {
					t.Errorf("expect event type %s, but got %s", tc.eventType, eType)
				}

				if !reflect.DeepEqual(tc.expectedObj, resObj) {
					t.Errorf("expect object \n%#+v\n, but got \n%#+v\n", tc.expectedObj, resObj)
				}
			}

			newReadCloser.Close()
		})
	}
}
