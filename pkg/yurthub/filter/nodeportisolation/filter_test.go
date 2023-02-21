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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func TestName(t *testing.T) {
	nif := &nodePortIsolationFilter{}
	if nif.Name() != filter.NodePortIsolationName {
		t.Errorf("expect %s, but got %s", filter.NodePortIsolationName, nif.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	nif := &nodePortIsolationFilter{}
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

func TestSetWorkingMode(t *testing.T) {
	nif := &nodePortIsolationFilter{}
	if err := nif.SetWorkingMode(hubutil.WorkingMode("cloud")); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}

	if nif.workingMode != hubutil.WorkingModeCloud {
		t.Errorf("expect working mode: cloud, but got %s", nif.workingMode)
	}
}

func TestSetSharedInformerFactory(t *testing.T) {
	client := &fake.Clientset{}
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	nif := &nodePortIsolationFilter{
		workingMode: "cloud",
	}
	if err := nif.SetSharedInformerFactory(informerFactory); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}
}

func TestSetStorageWrapper(t *testing.T) {
	nif := &nodePortIsolationFilter{
		workingMode: "edge",
		nodeName:    "foo",
	}
	storageManager, err := disk.NewDiskStorage("/tmp/nif-filter")
	if err != nil {
		t.Fatalf("could not create storage manager, %v", err)
	}
	storageWrapper := cachemanager.NewStorageWrapper(storageManager)

	if err := nif.SetStorageWrapper(storageWrapper); err != nil {
		t.Errorf("expect nil, but got %v", err)
	}
}

func TestFilter(t *testing.T) {
	nodePoolName := "foo"
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}
	testcases := map[string]struct {
		isOrphanNodes bool
		responseObj   runtime.Object
		expectObj     runtime.Object
	}{
		"enable NodePort service listening on nodes in foo and bar NodePool.": {
			responseObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "foo, bar",
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
			expectObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "foo, bar",
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
		"enable NodePort service listening on nodes of all NodePools": {
			responseObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "foo, *",
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
			expectObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "foo, *",
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
		"disable NodePort service listening on nodes of all NodePools": {
			responseObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "-foo,-bar",
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
								ServiceAnnotationNodePortListen: "-foo",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			expectObj: &corev1.ServiceList{},
		},
		"disable NodePort service listening only on nodes in foo NodePool": {
			responseObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
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
			expectObj: &corev1.ServiceList{
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
		"disable nodeport service": {
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
		"duplicated node pool configuration": {
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
		"disable NodePort service listening on nodes of foo NodePool": {
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
		"enable nodeport service on orphan nodes": {
			isOrphanNodes: true,
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
		"disable NodePort service listening if no value configured": {
			responseObj: &corev1.ServiceList{
				Items: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "svc1",
							Namespace: "default",
							Annotations: map[string]string{
								ServiceAnnotationNodePortListen: "",
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
								ServiceAnnotationNodePortListen: " ",
							},
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.96.105.188",
							Type:      corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			expectObj: &corev1.ServiceList{},
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

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			nif := &nodePortIsolationFilter{}
			if !tc.isOrphanNodes {
				nif.nodePoolName = nodePoolName
			} else {
				nif.nodeName = "foo"
				nif.nodeGetter = func(name string) (*corev1.Node, error) {
					return node, nil
				}
			}
			nif.nodeSynced = func() bool {
				return true
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
