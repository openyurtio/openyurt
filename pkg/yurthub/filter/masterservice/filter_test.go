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
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

func TestName(t *testing.T) {
	msf := &masterServiceFilter{}
	if msf.Name() != filter.MasterServiceFilterName {
		t.Errorf("expect %s, but got %s", filter.MasterServiceFilterName, msf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	msf := masterServiceFilter{}
	rvs := msf.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported more than one resources, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "services" {
			t.Errorf("expect resource is services, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("list", "watch")) {
			t.Errorf("expect verbs are list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestFilter(t *testing.T) {
	msf := &masterServiceFilter{
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
							ClusterIP: msf.host,
							Ports: []corev1.ServicePort{
								{
									Port: msf.port,
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
					ClusterIP: msf.host,
					Ports: []corev1.ServicePort{
						{
							Port: msf.port,
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

	stopCh := make(<-chan struct{})
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			newObj := msf.Filter(tt.responseObject, stopCh)
			if tt.expectObject == nil {
				if !util.IsNil(newObj) {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tt.expectObject) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectObject, newObj)
			}
		})
	}
}
