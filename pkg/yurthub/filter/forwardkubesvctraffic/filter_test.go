/*
Copyright 2024 The OpenYurt Authors.

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

package forwardkubesvctraffic

import (
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

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
	fkst, _ := NewForwardKubeSVCTrafficFilter()
	if fkst.Name() != FilterName {
		t.Errorf("expect %s, but got %s", FilterName, fkst.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	fkst, _ := NewForwardKubeSVCTrafficFilter()
	rvs := fkst.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported more than one resources, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "endpointslices" {
			t.Errorf("expect resource is endpointslices, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("list", "watch")) {
			t.Errorf("expect verbs are list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestFilter(t *testing.T) {
	portName := "https"

	readyCondition := pointer.Bool(true)
	var kasPort, masterPort int32
	kasPort = 6443
	masterHost := "169.251.2.1"
	masterPort = 10268

	testcases := map[string]struct {
		host           string
		port           string
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
		"endpointslice is kubernetes endpointslice": {
			host: masterHost,
			port: fmt.Sprintf("%d", masterPort),
			responseObject: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeSVCName,
					Namespace: KubeSVCNamespace,
				},
				AddressType: discovery.AddressTypeIPv4,
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{"172.16.0.14"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
						},
					},
					{
						Addresses: []string{"172.16.0.15"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
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
			expectObject: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      KubeSVCName,
					Namespace: KubeSVCNamespace,
				},
				AddressType: discovery.AddressTypeIPv4,
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{masterHost},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
						},
					},
				},
				Ports: []discovery.EndpointPort{
					{
						Name: &portName,
						Port: &masterPort,
					},
				},
			},
		},
		"endpointslice is not kubernetes endpointslice": {
			host: masterHost,
			port: fmt.Sprintf("%d", masterPort),
			responseObject: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: KubeSVCNamespace,
				},
				AddressType: discovery.AddressTypeIPv4,
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{"172.16.0.14"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
						},
					},
					{
						Addresses: []string{"172.16.0.15"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
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
			expectObject: &discovery.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: KubeSVCNamespace,
				},
				AddressType: discovery.AddressTypeIPv4,
				Endpoints: []discovery.Endpoint{
					{
						Addresses: []string{"172.16.0.14"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
						},
					},
					{
						Addresses: []string{"172.16.0.15"},
						Conditions: discovery.EndpointConditions{
							Ready: readyCondition,
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
		"it is not an endpointslice": {
			host: masterHost,
			port: fmt.Sprintf("%d", masterPort),
			responseObject: &corev1.PodList{
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
			expectObject: &corev1.PodList{
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
			fkst, _ := NewForwardKubeSVCTrafficFilter()
			fkst.SetMasterServiceHost(tt.host)
			fkst.SetMasterServicePort(tt.port)
			newObj := fkst.Filter(tt.responseObject, stopCh)
			if tt.expectObject == nil {
				if !util.IsNil(newObj) {
					t.Errorf("Filter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tt.expectObject) {
				t.Errorf("Filter got error, expected: \n%v\nbut got: \n%v\n", tt.expectObject, newObj)
			}
		})
	}
}
