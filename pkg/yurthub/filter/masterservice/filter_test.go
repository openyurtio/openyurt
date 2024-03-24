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
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

func TestRegister(t *testing.T) {
	filters := base.NewFilters([]string{})
	Register(filters)
	if !filters.Enabled(FilterName) {
		t.Errorf("couldn't register %s filter", FilterName)
	}
}

func TestName(t *testing.T) {
	msf, _ := NewMasterServiceFilter()
	if msf.Name() != FilterName {
		t.Errorf("expect %s, but got %s", FilterName, msf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	msf, _ := NewMasterServiceFilter()
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
	masterServiceHost := "169.251.2.1"
	var masterServicePort int32
	masterServicePort = 10268
	masterServicePortStr := "10268"

	testcases := map[string]struct {
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
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
					ClusterIP: masterServiceHost,
					Ports: []corev1.ServicePort{
						{
							Port: masterServicePort,
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
			msf := &masterServiceFilter{}
			msf.SetMasterServiceHost(masterServiceHost)
			msf.SetMasterServicePort(masterServicePortStr)
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
