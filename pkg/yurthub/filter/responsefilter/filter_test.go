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
	"time"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/forwardkubesvctraffic"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/inclusterconfig"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	LabelNodePoolName = "openyurt.io/pool-name"
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
	currentNodeName := "node1"
	nodeName2 := "node2"
	nodeName3 := "node3"
	poolName := "foo"
	masterHost := "169.254.2.1"
	masterPort := "10268"
	var masterPortInt int32
	masterPortInt = 10268
	readyCondition := true
	portName := "https"
	var kasPort int32
	kasPort = 443
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	nodeBucketGVRToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1alpha1", Resource: "nodebuckets"}: "NodeBucketList",
	}
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1beta1", Resource: "nodepools"}: "NodePoolList",
	}
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		poolName                  string
		masterHost                string
		masterPort                string
		enableNodePool            bool
		enablePoolServiceTopology bool
		nodeName                  string
		kubeClient                *k8sfake.Clientset
		yurtClient                *fake.FakeDynamicClient
		group                     string
		version                   string
		resource                  string
		userAgent                 string
		verb                      string
		path                      string
		accept                    string
		inputObj                  runtime.Object
		expectedObj               runtime.Object
	}{
		"discardcloudservice: discard lb service only": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
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
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
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
		"discardcloudservice: discard cloud clusterIP service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "x-tunnel-server-internal-svc",
							Namespace: "kube-system",
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
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
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
		"discardcloudservice: discard all service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "x-tunnel-server-internal-svc",
							Namespace: "kube-system",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{},
			},
		},
		"discardcloudservice: skip all service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								discardcloudservice.DiscardServiceAnnotation: "false",
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
			expectedObj: &corev1.ServiceList{
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
								discardcloudservice.DiscardServiceAnnotation: "false",
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
		"inclusterconfig: mutate kube-proxy configmap": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "configmaps",
			userAgent:  "kubelet",
			verb:       "GET",
			path:       "/api/v1/configmaps",
			accept:     "application/json",
			inputObj: &corev1.ConfigMapList{
				Items: []corev1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      inclusterconfig.KubeProxyConfigMapName,
							Namespace: inclusterconfig.KubeProxyConfigMapNamespace,
						},
						Data: map[string]string{
							"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
						},
					},
				},
			},
			expectedObj: &corev1.ConfigMapList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMapList",
					APIVersion: "v1",
				},
				Items: []corev1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      inclusterconfig.KubeProxyConfigMapName,
							Namespace: inclusterconfig.KubeProxyConfigMapNamespace,
						},
						Data: map[string]string{
							"config.conf": "    apiVersion: kubeproxy.config.k8s.io/v1alpha1\n    bindAddress: 0.0.0.0\n    bindAddressHardFail: false\n    clientConnection:\n      acceptContentTypes: \"\"\n      burst: 0\n      contentType: \"\"\n      #kubeconfig: /var/lib/kube-proxy/kubeconfig.conf\n      qps: 0\n    clusterCIDR: 10.244.0.0/16\n    configSyncPeriod: 0s",
						},
					},
				},
			},
		},
		"masterservice: serviceList contains kubernetes service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kubelet",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      masterservice.MasterServiceName,
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.0.1",
							Ports: []corev1.ServicePort{
								{
									Port: 443,
									Name: masterservice.MasterServicePortName,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Ports: []corev1.ServicePort{
								{
									Port: 80,
								},
							},
						},
					},
				},
			},
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      masterservice.MasterServiceName,
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: masterHost,
							Ports: []corev1.ServicePort{
								{
									Port: masterPortInt,
									Name: masterservice.MasterServicePortName,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Ports: []corev1.ServicePort{
								{
									Port: 80,
								},
							},
						},
					},
				},
			},
		},
		"masterservice: serviceList doesn't contain kubernetes service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kubelet",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Ports: []corev1.ServicePort{
								{
									Port: 80,
								},
							},
						},
					},
				},
			},
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: masterservice.MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Ports: []corev1.ServicePort{
								{
									Port: 80,
								},
							},
						},
					},
				},
			},
		},
		"masterservice: it is a kubernetes service": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kubelet",
			verb:       "GET",
			path:       "/api/v1/namespaces/default/services/kubernetes",
			accept:     "application/json",
			inputObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      masterservice.MasterServiceName,
					Namespace: masterservice.MasterServiceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.188",
					Ports: []corev1.ServicePort{
						{
							Port: 80,
							Name: masterservice.MasterServicePortName,
						},
					},
				},
			},
			expectedObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      masterservice.MasterServiceName,
					Namespace: masterservice.MasterServiceNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: masterHost,
					Ports: []corev1.ServicePort{
						{
							Port: masterPortInt,
							Name: masterservice.MasterServicePortName,
						},
					},
				},
			},
		},
		"nodeportisolation: enable NodePort service listening on nodes in foo and bar NodePool": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			poolName:   poolName,
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "foo, bar",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
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
			expectedObj: &corev1.ServiceList{
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
								nodeportisolation.ServiceAnnotationNodePortListen: "foo, bar",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
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
		"nodeportisolation: enable NodePort service listening on nodes of all NodePools": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			poolName:   poolName,
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "foo, *",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
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
			expectedObj: &corev1.ServiceList{
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
								nodeportisolation.ServiceAnnotationNodePortListen: "foo, *",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
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
		"nodeportisolation: disable NodePort service listening on nodes of all NodePools": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			poolName:   poolName,
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "-foo,-bar",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "-foo",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{},
			},
		},
		"nodeportisolation: NodePort service doesn't listen on nodes in foo NodePool": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			poolName:   poolName,
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "-foo,*",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
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
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{
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
		"nodeportisolation: disable NodePort service listening if no value configured": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			poolName:   poolName,
			group:      "",
			version:    "v1",
			resource:   "services",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/api/v1/services",
			accept:     "application/json",
			inputObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: "",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.187",
							Type:      corev1.ServiceTypeNodePort,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc2",
							Namespace: "default",
							Annotations: map[string]string{
								nodeportisolation.ServiceAnnotationNodePortListen: " ",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			expectedObj: &corev1.ServiceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceList",
					APIVersion: "v1",
				},
				Items: []corev1.Service{},
			},
		},
		"servicetopology: v1beta1.EndpointSliceList: topologyKeys is kubernetes.io/hostname": {
			masterHost:                masterHost,
			masterPort:                masterPort,
			poolName:                  "hangzhou",
			nodeName:                  "node1",
			enablePoolServiceTopology: true,
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind,
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node3",
						},
					},
				},
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
						Labels: map[string]string{
							LabelNodePoolName: "shanghai",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node2",
						},
					},
				},
			),
			group:     "discovery.k8s.io",
			version:   "v1beta1",
			resource:  "endpointslices",
			userAgent: "kube-proxy",
			verb:      "GET",
			path:      "/apis/discovery.k8s.io/v1beta1/endpointslices",
			accept:    "application/json",
			inputObj: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discoveryV1beta1.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.3",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node2",
								},
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node3",
								},
							},
						},
					},
				},
			},
			expectedObj: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discoveryV1beta1.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
						},
					},
				},
			},
		},
		"servicetopology: v1beta1.EndpointSliceList: topologyKeys is openyurt.io/nodepool": {
			masterHost:     masterHost,
			masterPort:     masterPort,
			poolName:       "hangzhou",
			nodeName:       "node1",
			enableNodePool: true,
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
					Status: v1beta1.NodePoolStatus{
						Nodes: []string{
							"node1",
							"node3",
						},
					},
				},
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
					Status: v1beta1.NodePoolStatus{
						Nodes: []string{
							"node2",
						},
					},
				},
			),
			group:     "discovery.k8s.io",
			version:   "v1beta1",
			resource:  "endpointslices",
			userAgent: "kube-proxy",
			verb:      "GET",
			path:      "/apis/discovery.k8s.io/v1beta1/endpointslices",
			accept:    "application/json",
			inputObj: &discoveryV1beta1.EndpointSliceList{
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discoveryV1beta1.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.3",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node2",
								},
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node3",
								},
							},
						},
					},
				},
			},
			expectedObj: &discoveryV1beta1.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1beta1",
				},
				Items: []discoveryV1beta1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discoveryV1beta1.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discoveryV1beta1.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node1",
								},
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								Topology: map[string]string{
									corev1.LabelHostname: "node3",
								},
							},
						},
					},
				},
			},
		},
		"servicetopology: v1.EndpointSliceList: topologyKeys is kubernetes.io/hostname": {
			masterHost:                masterHost,
			masterPort:                masterPort,
			poolName:                  "hangzhou",
			nodeName:                  "node1",
			enablePoolServiceTopology: true,
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNode,
						},
					},
				},
			),
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind,
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node3",
						},
					},
				},
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
						Labels: map[string]string{
							LabelNodePoolName: "shanghai",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node2",
						},
					},
				},
			),
			group:     "discovery.k8s.io",
			version:   "v1",
			resource:  "endpointslices",
			userAgent: "kube-proxy",
			verb:      "GET",
			path:      "/apis/discovery.k8s.io/v1/endpointslices",
			accept:    "application/json",
			inputObj: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.3",
								},
								NodeName: &nodeName2,
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								NodeName: &currentNodeName,
							},
						},
					},
				},
			},
		},
		"servicetopology: v1.EndpointSliceList: topologyKeys is openyurt.io/nodepool": {
			masterHost:                masterHost,
			masterPort:                masterPort,
			poolName:                  "hangzhou",
			nodeName:                  "node1",
			enablePoolServiceTopology: true,
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind,
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node3",
						},
					},
				},
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
						Labels: map[string]string{
							LabelNodePoolName: "shanghai",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node2",
						},
					},
				},
			),
			group:     "discovery.k8s.io",
			version:   "v1",
			resource:  "endpointslices",
			userAgent: "kube-proxy",
			verb:      "GET",
			path:      "/apis/discovery.k8s.io/v1/endpointslices",
			accept:    "application/json",
			inputObj: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.3",
								},
								NodeName: &nodeName2,
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1-np7sf",
							Namespace: "default",
							Labels: map[string]string{
								discovery.LabelServiceName: "svc1",
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{
									"10.244.1.2",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.4",
								},
								NodeName: &currentNodeName,
							},
							{
								Addresses: []string{
									"10.244.1.5",
								},
								NodeName: &nodeName3,
							},
						},
					},
				},
			},
		},
		"servicetopology: v1.EndpointSliceList: there are no endpoints": {
			masterHost:                masterHost,
			masterPort:                masterPort,
			poolName:                  "hangzhou",
			nodeName:                  "node1",
			enablePoolServiceTopology: true,
			kubeClient: k8sfake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNodePool,
						},
					},
				},
			),
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind,
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node3",
						},
					},
				},
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "shanghai",
						Labels: map[string]string{
							LabelNodePoolName: "shanghai",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node2",
						},
					},
				},
			),
			group:     "discovery.k8s.io",
			version:   "v1",
			resource:  "endpointslices",
			userAgent: "kube-proxy",
			verb:      "GET",
			path:      "/apis/discovery.k8s.io/v1/endpointslices",
			accept:    "application/json",
			inputObj:  &discovery.EndpointSliceList{},
			expectedObj: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
			},
		},
		"forwardkubesvctraffic: endpointsliceList contains kubernetes endpointslice": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "discovery.k8s.io",
			version:    "v1",
			resource:   "endpointslices",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/apis/discovery.k8s.io/v1/endpointslices",
			accept:     "application/json",
			inputObj: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      forwardkubesvctraffic.KubeSVCName,
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.14"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.15"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.16"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.17"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      forwardkubesvctraffic.KubeSVCName,
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{masterHost},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &masterPortInt,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.16"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.17"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
				},
			},
		},
		"forwardkubesvctraffic: endpointsliceList doesn't contain kubernetes endpointslice": {
			masterHost: masterHost,
			masterPort: masterPort,
			kubeClient: &k8sfake.Clientset{},
			yurtClient: &fake.FakeDynamicClient{},
			group:      "discovery.k8s.io",
			version:    "v1",
			resource:   "endpointslices",
			userAgent:  "kube-proxy",
			verb:       "GET",
			path:       "/apis/discovery.k8s.io/v1/endpointslices",
			accept:     "application/json",
			inputObj: &discovery.EndpointSliceList{
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.14"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.15"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.16"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.17"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
				},
			},
			expectedObj: &discovery.EndpointSliceList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "EndpointSliceList",
					APIVersion: "discovery.k8s.io/v1",
				},
				Items: []discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.14"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.15"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: forwardkubesvctraffic.KubeSVCNamespace,
						},
						AddressType: discovery.AddressTypeIPv4,
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"172.16.0.16"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
							{
								Addresses: []string{"172.16.0.17"},
								Conditions: discovery.EndpointConditions{
									Ready: &readyCondition,
								},
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: &portName,
								Port: &kasPort,
							},
						},
					},
				},
			},
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			var factory informers.SharedInformerFactory
			var yurtFactory dynamicinformer.DynamicSharedInformerFactory
			var nodesInitializer filter.Initializer
			var genericInitializer filter.Initializer

			factory = informers.NewSharedInformerFactory(tc.kubeClient, 24*time.Hour)
			yurtFactory = dynamicinformer.NewDynamicSharedInformerFactory(tc.yurtClient, 24*time.Hour)

			nodesInitializer = initializer.NewNodesInitializer(tc.enableNodePool, tc.enablePoolServiceTopology, yurtFactory)
			genericInitializer = initializer.New(factory, tc.kubeClient, tc.nodeName, tc.poolName, tc.masterHost, tc.masterPort)
			initializerChain := base.Initializers{}
			initializerChain = append(initializerChain, genericInitializer, nodesInitializer)

			// get all object filters
			baseFilters := base.NewFilters([]string{})
			options.RegisterAllFilters(baseFilters)

			objectFilters, err := baseFilters.NewFromFilters(initializerChain)
			if err != nil {
				t.Errorf("couldn't new object filters, %v", err)
			}

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

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			stopper2 := make(chan struct{})
			defer close(stopper2)
			yurtFactory.Start(stopper2)
			yurtFactory.WaitForCacheSync(stopper2)

			var newReadCloser io.ReadCloser
			var filterErr error
			var responseFilter filter.ResponseFilter
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				responseFilter = CreateResponseFilter(objectFilters, serializerManager)
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
		objectFilters     []filter.ObjectFilter
		group             string
		version           string
		resource          string
		userAgent         string
		verb              string
		path              string
		accept            string
		eventType         watch.EventType
		inputObj          runtime.Object
		names             sets.String
		expectedObj       runtime.Object
		expectedEventType watch.EventType
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
			names: sets.NewString("discardcloudservice"),
			expectedObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
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
			expectedEventType: watch.Deleted,
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
			names: sets.NewString("discardcloudservice", "nodeportisolation"),
			expectedObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
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
			expectedEventType: watch.Deleted,
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
			expectedEventType: watch.Added,
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

				if eType != tc.expectedEventType {
					t.Errorf("expect event type %s, but got %s", tc.expectedEventType, eType)
				}

				if !reflect.DeepEqual(tc.expectedObj, resObj) {
					t.Errorf("expect object \n%#+v\n, but got \n%#+v\n", tc.expectedObj, resObj)
				}
			}

			newReadCloser.Close()
		})
	}
}
