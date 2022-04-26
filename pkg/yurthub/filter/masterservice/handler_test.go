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

package masterservice

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

func TestObjectResponseFilter(t *testing.T) {
	fh := &masterServiceFilterHandler{
		host: "169.251.2.1",
		port: 10268,
	}

	testcases := map[string]struct {
		group        string
		version      string
		resources    string
		userAgent    string
		accept       string
		verb         string
		path         string
		originalList runtime.Object
		expectResult runtime.Object
	}{
		"serviceList contains kubernetes service": {
			group:     "",
			version:   "v1",
			resources: "services",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/services",
			originalList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      MasterServiceName,
							Namespace: MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.0.1",
							Ports: []corev1.ServicePort{
								{
									Port: 443,
									Name: MasterServicePortName,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: MasterServiceNamespace,
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
			expectResult: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      MasterServiceName,
							Namespace: MasterServiceNamespace,
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: fh.host,
							Ports: []corev1.ServicePort{
								{
									Port: fh.port,
									Name: MasterServicePortName,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: MasterServiceNamespace,
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
		"serviceList does not contain kubernetes service": {
			group:     "",
			version:   "v1",
			resources: "services",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/services",
			originalList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: MasterServiceNamespace,
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
			expectResult: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: MasterServiceNamespace,
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
		"not serviceList": {
			group:     "",
			version:   "v1",
			resources: "pods",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/pods",
			originalList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: MasterServiceNamespace,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
			expectResult: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: MasterServiceNamespace,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			req.Header.Set("User-Agent", tt.userAgent)
			req.Header.Set("Accept", tt.accept)
			fh.req = req
			fh.serializer = serializer.NewSerializerManager().
				CreateSerializer(tt.accept, tt.group, tt.version, tt.resources)

			originalBytes, err := fh.serializer.Encode(tt.originalList)
			if err != nil {
				t.Errorf("encode originalList error: %v\n", err)
			}

			filteredBytes, err := fh.ObjectResponseFilter(originalBytes)
			if err != nil {
				t.Errorf("ObjectResponseFilter got error: %v\n", err)
			}

			expectedBytes, err := fh.serializer.Encode(tt.expectResult)
			if err != nil {
				t.Errorf("encode expectedResult error: %v\n", err)
			}

			if !bytes.Equal(filteredBytes, expectedBytes) {
				result, _ := fh.serializer.Decode(filteredBytes)
				t.Errorf("ObjectResponseFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult, result)
			}
		})
	}
}

func TestStreamResponseFilter(t *testing.T) {
	fh := &masterServiceFilterHandler{
		host: "169.251.2.1",
		port: 10268,
	}

	testcases := map[string]struct {
		group        string
		version      string
		resources    string
		userAgent    string
		accept       string
		verb         string
		path         string
		inputObj     []watch.Event
		expectResult []runtime.Object
	}{
		"watch kubernetes service": {
			group:     "",
			version:   "v1",
			resources: "services",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/services?watch=true",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      MasterServiceName,
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.0.1",
						Ports: []corev1.ServicePort{
							{
								Port: 443,
								Name: MasterServicePortName,
							},
						},
					},
				}},
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.188",
						Ports: []corev1.ServicePort{
							{
								Port: 80,
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      MasterServiceName,
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: fh.host,
						Ports: []corev1.ServicePort{
							{
								Port: fh.port,
								Name: MasterServicePortName,
							},
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: MasterServiceNamespace,
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
		"watch without kubernetes service": {
			group:     "",
			version:   "v1",
			resources: "services",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/services?watch=true",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.188.105",
						Ports: []corev1.ServicePort{
							{
								Port: 80,
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.188.105",
						Ports: []corev1.ServicePort{
							{
								Port: 80,
							},
						},
					},
				},
			},
		},
		"watch pods": {
			group:     "",
			version:   "v1",
			resources: "pods",
			userAgent: "kubelet",
			accept:    "application/json",
			verb:      "GET",
			path:      "/api/v1/namespaces/default/pods?watch=true",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				}},
			},
			expectResult: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: MasterServiceNamespace,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			req.Header.Set("User-Agent", tt.userAgent)
			req.Header.Set("Accept", tt.accept)
			fh.req = req

			fh.serializer = serializer.NewSerializerManager().
				CreateSerializer(tt.accept, tt.group, tt.version, tt.resources)

			r, w := io.Pipe()
			go func(w *io.PipeWriter) {
				for i := range tt.inputObj {
					if _, err := fh.serializer.WatchEncode(w, &tt.inputObj[i]); err != nil {
						t.Errorf("%d: encode watch unexpected error: %v", i, err)
						continue
					}
					time.Sleep(100 * time.Millisecond)
				}
				w.Close()
			}(w)

			rc := io.NopCloser(r)
			ch := make(chan watch.Event, len(tt.inputObj))

			go func(rc io.ReadCloser, ch chan watch.Event) {
				fh.StreamResponseFilter(rc, ch)
			}(rc, ch)

			for i := 0; i < len(tt.expectResult); i++ {
				event := <-ch

				resultBytes, _ := fh.serializer.Encode(event.Object)
				expectedBytes, _ := fh.serializer.Encode(tt.expectResult[i])

				if !bytes.Equal(resultBytes, expectedBytes) {
					t.Errorf("StreamResponseFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectResult[i], event.Object)
					break
				}
			}
		})
	}
}
