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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRuntimeObjectFilter(t *testing.T) {
	fh := &masterServiceFilterHandler{
		host: "169.251.2.1",
		port: 10268,
	}

	testcases := map[string]struct {
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
		"serviceList contains kubernetes service": {
			responseObject: &corev1.ServiceList{
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
			expectObject: &corev1.ServiceList{
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
			responseObject: &corev1.ServiceList{
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
			expectObject: &corev1.ServiceList{
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
		"it's a kubernetes service": {
			responseObject: &corev1.Service{
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
			expectObject: &corev1.Service{
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
		},
		"it's not a kubernetes service": {
			responseObject: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
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
			expectObject: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
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
		},
		"not serviceList": {
			responseObject: &corev1.PodList{
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
			expectObject: &corev1.PodList{
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
			newObj, isNil := fh.RuntimeObjectFilter(tt.responseObject)
			if tt.expectObject == nil {
				if !isNil {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tt.expectObject) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n, isNil=%v", tt.expectObject, newObj, isNil)
			}
		})
	}
}
