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

package filter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"

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

func registerAllFilters(filters *Filters) {
	filters.Register(ServiceTopologyFilterName, func() (ObjectFilter, error) {
		return &nopObjectHandler{name: ServiceTopologyFilterName}, nil
	})
	filters.Register(DiscardCloudServiceFilterName, func() (ObjectFilter, error) {
		return &nopObjectHandler{name: DiscardCloudServiceFilterName}, nil
	})
	filters.Register(MasterServiceFilterName, func() (ObjectFilter, error) {
		return &nopObjectHandler{name: MasterServiceFilterName}, nil
	})
}

type nopInitializer struct{}

func (nopInit *nopInitializer) Initialize(_ ObjectFilter) error {
	return nil
}

func TestNewFromFilters(t *testing.T) {
	allFilters := []string{MasterServiceFilterName, DiscardCloudServiceFilterName, ServiceTopologyFilterName}
	testcases := map[string]struct {
		disabledFilters  []string
		generatedFilters sets.String
	}{
		"disable master service filter": {
			disabledFilters:  []string{MasterServiceFilterName},
			generatedFilters: sets.NewString(allFilters...).Delete(MasterServiceFilterName),
		},
		"disable service topology filter": {
			disabledFilters:  []string{ServiceTopologyFilterName},
			generatedFilters: sets.NewString(allFilters...).Delete(ServiceTopologyFilterName),
		},
		"disable discard cloud service filter": {
			disabledFilters:  []string{DiscardCloudServiceFilterName},
			generatedFilters: sets.NewString(allFilters...).Delete(DiscardCloudServiceFilterName),
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			filters := NewFilters(tt.disabledFilters)
			registerAllFilters(filters)

			runners, err := filters.NewFromFilters(&nopInitializer{})
			if err != nil {
				t.Errorf("failed to new from filters, %v", err)
			}

			gotRunners := sets.NewString()
			for i := range runners {
				gotRunners.Insert(runners[i].Name())
			}

			if !gotRunners.Equal(tt.generatedFilters) {
				t.Errorf("expect filters %v, but got %v", tt.generatedFilters, gotRunners)
			}
		})
	}
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
							Annotations: map[string]string{
								SkipDiscardServiceAnnotation: "true",
							},
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
							Annotations: map[string]string{
								SkipDiscardServiceAnnotation: "true",
							},
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
							Annotations: map[string]string{
								SkipDiscardServiceAnnotation: "true",
							},
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
							Annotations: map[string]string{
								SkipDiscardServiceAnnotation: "true",
							},
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
				s := CreateSerializer(reqContentType, info, sm)

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
					Annotations: map[string]string{
						SkipDiscardServiceAnnotation: "true",
					},
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
					Annotations: map[string]string{
						SkipDiscardServiceAnnotation: "true",
					},
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
					Annotations: map[string]string{
						SkipDiscardServiceAnnotation: "true",
					},
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
					Annotations: map[string]string{
						SkipDiscardServiceAnnotation: "true",
					},
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
				s := CreateSerializer(reqContentType, info, sm)

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
