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

package discardcloudservice

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

func TestObjectResponseFilter(t *testing.T) {
	testcases := map[string]struct {
		group        string
		version      string
		resources    string
		accept       string
		originalList runtime.Object
		expectResult runtime.Object
	}{
		"serviceList contains LoadBalancer service with SkipDiscardServiceAnnotation is not true": {
			group:     "",
			version:   "v1",
			resources: "services",
			accept:    "application/json",
			originalList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "false",
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
			expectResult: &corev1.ServiceList{
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
		"serviceList contains LoadBalancer service, but SkipDiscardServiceAnnotation is true": {
			group:     "",
			version:   "v1",
			resources: "services",
			accept:    "application/json",
			originalList: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
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
			expectResult: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								filter.SkipDiscardServiceAnnotation: "true",
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
		"not serviceList": {
			group:     "",
			version:   "v1",
			resources: "pods",
			accept:    "application/json",
			originalList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: "default",
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
							Namespace: "default",
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
			fh := &discardCloudServiceFilterHandler{
				serializer: serializer.NewSerializerManager().
					CreateSerializer(tt.accept, tt.group, tt.version, tt.resources),
			}

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
	testcases := map[string]struct {
		group        string
		version      string
		resources    string
		accept       string
		inputObj     []watch.Event
		expectResult []runtime.Object
	}{
		"watch services that contain LoadBalancer service with SkipDiscardServiceAnnotation is not true": {
			group:     "",
			version:   "v1",
			resources: "services",
			accept:    "application/json",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "false",
						},
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.187",
						Type:      corev1.ServiceTypeLoadBalancer,
					},
				}},
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc2",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.188",
						Type:      corev1.ServiceTypeClusterIP,
					},
				}},
			},
			expectResult: []runtime.Object{
				&corev1.Service{
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
		"watch services that contain LoadBalancer service, but SkipDiscardServiceAnnotation is true": {
			group:     "",
			version:   "v1",
			resources: "services",
			accept:    "application/json",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.187",
						Type:      corev1.ServiceTypeLoadBalancer,
					},
				}},
				{Type: watch.Modified, Object: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc2",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.188",
						Type:      corev1.ServiceTypeClusterIP,
					},
				}},
			},
			expectResult: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc1",
						Namespace: "default",
						Annotations: map[string]string{
							filter.SkipDiscardServiceAnnotation: "true",
						},
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: "10.96.105.187",
						Type:      corev1.ServiceTypeLoadBalancer,
					},
				},
				&corev1.Service{
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
		"watch pods": {
			group:     "",
			version:   "v1",
			resources: "services",
			accept:    "application/json",
			inputObj: []watch.Event{
				{Type: watch.Modified, Object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
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
						Namespace: "default",
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
			fh := &discardCloudServiceFilterHandler{
				serializer: serializer.NewSerializerManager().
					CreateSerializer(tt.accept, tt.group, tt.version, tt.resources),
			}

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

			rc := ioutil.NopCloser(r)
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
