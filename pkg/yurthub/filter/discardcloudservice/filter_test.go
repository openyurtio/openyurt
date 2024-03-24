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
	dcsf, _ := NewDiscardCloudServiceFilter()
	if dcsf.Name() != FilterName {
		t.Errorf("expect %s, but got %s", FilterName, dcsf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	dcsf, _ := NewDiscardCloudServiceFilter()
	rvs := dcsf.SupportedResourceAndVerbs()
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
	testcases := map[string]struct {
		responseObj runtime.Object
		expectObj   runtime.Object
	}{
		"discard lb service": {
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						DiscardServiceAnnotation: "true",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			expectObj: nil,
		},
		"discard cloud clusterIP service": {
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeClusterIP,
				},
			},
			expectObj: nil,
		},
		"skip lb service": {
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeLoadBalancer,
				},
			},
			expectObj: &corev1.Service{
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
		"skip podList": {
			responseObj: &corev1.PodList{
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
			expectObj: &corev1.PodList{
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

	stopCh := make(<-chan struct{})
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dcsf, _ := NewDiscardCloudServiceFilter()

			newObj := dcsf.Filter(tt.responseObj, stopCh)
			if tt.expectObj == nil {
				if !util.IsNil(newObj) {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tt.expectObj) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n", tt.expectObj, newObj)
			}
		})
	}
}
