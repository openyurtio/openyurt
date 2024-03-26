/*
Copyright 2023 The OpenYurt Authors.

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

package nodeportisolation

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
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
	nif, _ := NewNodePortIsolationFilter()
	if nif.Name() != FilterName {
		t.Errorf("expect %s, but got %s", FilterName, nif.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	nif, _ := NewNodePortIsolationFilter()
	rvs := nif.SupportedResourceAndVerbs()
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

func TestSetNodePoolName(t *testing.T) {
	nif := &nodePortIsolationFilter{}
	if err := nif.SetNodePoolName("nodepool1"); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}

	if nif.nodePoolName != "nodepool1" {
		t.Errorf("expect nodepool name: nodepool1, but got %s", nif.nodePoolName)
	}
}

func TestSetNodeName(t *testing.T) {
	nif := &nodePortIsolationFilter{}
	if err := nif.SetNodeName("foo"); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}

	if nif.nodeName != "foo" {
		t.Errorf("expect node name: foo, but got %s", nif.nodePoolName)
	}
}

func TestSetKubeClient(t *testing.T) {
	client := &fake.Clientset{}
	nif := &nodePortIsolationFilter{}
	if err := nif.SetKubeClient(client); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}
}

func TestFilter(t *testing.T) {
	nodePoolName := "foo"
	nodeFoo := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Annotations: map[string]string{
				projectinfo.GetNodePoolLabel(): nodePoolName,
			},
		},
	}
	nodeBar := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
	}
	testcases := map[string]struct {
		poolName    string
		nodeName    string
		responseObj runtime.Object
		expectObj   runtime.Object
	}{
		"disable nodeport service": {
			poolName: nodePoolName,
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: nil,
		},
		"disable NodePort service listening on nodes of foo NodePool": {
			poolName: nodePoolName,
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: nil,
		},
		"disable NodePort service listening on nodes of bar NodePool": {
			poolName: "bar",
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: nil,
		},
		"duplicated node pool configuration": {
			nodeName: "foo",
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "foo,-foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "foo,-foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
		},
		"enable NodePort service listening on nodes of foo NodePool": {
			poolName: nodePoolName,
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "foo",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
		},
		"enable NodePort service listening on all nodes": {
			poolName: nodePoolName,
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
		},
		"enable nodeport service on orphan nodes": {
			nodeName: "bar",
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo,*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo,*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
		},
		"enable nodeport service on orphan node that can not found": {
			nodeName: "notfound",
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo,*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
					Annotations: map[string]string{
						ServiceAnnotationNodePortListen: "-foo,*",
					},
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeNodePort,
				},
			},
		},
		"skip ClusterIP service": {
			poolName: nodePoolName,
			responseObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeClusterIP,
				},
			},
			expectObj: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.105.187",
					Type:      corev1.ServiceTypeClusterIP,
				},
			},
		},
		"skip podList": {
			poolName: nodePoolName,
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

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			nif := &nodePortIsolationFilter{}
			if len(tc.poolName) != 0 {
				nif.nodePoolName = tc.poolName
			}

			if len(tc.nodeName) != 0 {
				nif.nodeName = tc.nodeName
				client := fake.NewSimpleClientset(nodeFoo, nodeBar)
				nif.client = client
			}

			newObj := nif.Filter(tc.responseObj, stopCh)
			if tc.expectObj == nil {
				if !util.IsNil(newObj) {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tc.expectObj) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n", tc.expectObj, newObj)
			}
		})
	}
}
