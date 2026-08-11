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
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// TestResolveNodePoolNameConcurrently ensures that the node pool name cached on the shared
// filter instance can be resolved from several goroutines at once. The filter manager hands
// the same nodePortIsolationFilter to every request, so Filter runs concurrently whenever
// more than one component is listing or watching services. Run with -race to catch a
// regression.
func TestResolveNodePoolNameConcurrently(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node1",
				Labels: map[string]string{projectinfo.GetNodePoolLabel(): "hangzhou"},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node2",
				Labels: map[string]string{projectinfo.GetNodePoolLabel(): "shanghai"},
			},
		},
	)
	// node pool name is deliberately left unset, so that every Filter call has to resolve it
	// from the node object. This is the default, because --nodepool-name is optional.
	nif := &nodePortIsolationFilter{nodeName: "node1", client: client}

	// kept: hangzhou is listed by the annotation. discarded: only shanghai is listed.
	keptSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "kept",
			Namespace:   "default",
			Annotations: map[string]string{ServiceAnnotationNodePortListen: "hangzhou"},
		},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
	}
	discardedSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "discarded",
			Namespace:   "default",
			Annotations: map[string]string{ServiceAnnotationNodePortListen: "shanghai"},
		},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
	}

	stopCh := make(<-chan struct{})
	kept := make([]runtime.Object, 50)
	discarded := make([]runtime.Object, 50)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			kept[i] = nif.Filter(keptSvc.DeepCopy(), stopCh)
		}(i)
		go func(i int) {
			defer wg.Done()
			discarded[i] = nif.Filter(discardedSvc.DeepCopy(), stopCh)
		}(i)
	}
	wg.Wait()

	for i := range kept {
		if util.IsNil(kept[i]) {
			t.Errorf("goroutine %d: service listened by nodepool hangzhou should have been kept", i)
		}
		if !util.IsNil(discarded[i]) {
			t.Errorf("goroutine %d: service listened only by nodepool shanghai should have been discarded, but got %v", i, discarded[i])
		}
	}

	if poolName := nif.resolveNodePoolName(); poolName != "hangzhou" {
		t.Errorf("expect cached node pool name hangzhou, but got %q", poolName)
	}
}
