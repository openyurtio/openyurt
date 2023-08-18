/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package podreadyupdater

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/constant"
)

var (
	TestPodsName   = []string{"pod1", "pod2", "pod3", "pod4", "pod5", "pod6", "pod7", "pod8", "pod9"}
	TestNodesName  = []string{"node1"}
	TestNodesName2 = []string{"node1", "node2"}
	TestFailedPods = sets.NewString("pod2", "pod3", "pod4", "pod5", "pod6", "pod7", "pod8", "pod9")
)

func preparePods() []client.Object {
	pods := []client.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node1",
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node2",
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod3",
				Namespace:         metav1.NamespaceDefault,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod4",
				Namespace: metav1.NamespaceDefault,
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod5",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node5",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod6",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node2",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod7",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node3",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod8",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				NodeName: "node4",
			},
		},
	}

	return pods
}

func prepareNodes() []client.Object {
	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					apps.NodePoolLabel: "nodePool1",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Annotations: map[string]string{
					constant.PodBindingAnnotation: "true",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
				Labels: map[string]string{
					apps.NodePoolLabel: "nodePool3",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node4",
				Labels: map[string]string{
					apps.NodePoolLabel: "nodePool2",
				},
			},
		},
	}
	return nodes
}

func prepareNodePools() []client.Object {
	nodePools := []client.Object{
		&appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nodePool1",
			},
			Status: appsv1beta1.NodePoolStatus{
				ReadyNodeNum:   0,
				UnreadyNodeNum: 1,
				Nodes:          TestNodesName,
			},
		},
		&appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nodePool2",
			},
			Status: appsv1beta1.NodePoolStatus{
				ReadyNodeNum:   0,
				UnreadyNodeNum: 1,
				Nodes:          TestNodesName2,
			},
		},
	}

	return nodePools
}

func TestReconcile(t *testing.T) {
	pods := preparePods()
	nodes := prepareNodes()
	nodePools := prepareNodePools()

	scheme := runtime.NewScheme()
	if err := appsv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add yurt custom resource")
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}

	c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(pods...).WithObjects(nodes...).WithObjects(nodePools...).Build()

	for i := range TestPodsName {
		var req = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: TestPodsName[i]}}
		rsp := ReconcilePodReadyUpdater{
			Client: c,
			scheme: scheme,
		}

		_, err := rsp.Reconcile(context.TODO(), req)
		if err != nil {
			if !TestFailedPods.Has(TestPodsName[i]) {
				t.Fatalf("failed to control podReadyUpdater controller")
			}
			continue
		}
		pod := &corev1.Pod{}
		err = c.Get(context.TODO(), types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: TestPodsName[i]}, pod)
		if err != nil {
			if !TestFailedPods.Has(TestPodsName[i]) {
				t.Fatalf("failed to get pod")
			}
			continue
		}
		if !podutil.IsPodReady(pod) && !TestFailedPods.Has(TestPodsName[i]) {
			t.Errorf("pod %s ready condition want true", pod.Name)
		}
	}
}

func TestNodePoolUpdate(t *testing.T) {
	tests := []struct {
		name  string
		event event.UpdateEvent
		want  bool
	}{
		{
			name: "test1",
			event: event.UpdateEvent{
				ObjectOld: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool1",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
				ObjectNew: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool1",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   0,
						UnreadyNodeNum: 1,
						Nodes:          TestNodesName,
					},
				},
			},
			want: true,
		}, {
			name: "test2",
			event: event.UpdateEvent{
				ObjectOld: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool2",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
				ObjectNew: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool2",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
			},
			want: false,
		}, {
			name: "test3",
			event: event.UpdateEvent{
				ObjectOld: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool2",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			want: false,
		}, {
			name: "test4",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
				ObjectNew: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool2",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
			},
			want: false,
		}, {
			name: "test5",
			event: event.UpdateEvent{
				ObjectOld: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool1",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName2,
					},
				},
				ObjectNew: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool1",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   0,
						UnreadyNodeNum: 1,
						Nodes:          TestNodesName2,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodePoolUpdate(tt.event); got != tt.want {
				t.Errorf("nodePoolUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkPodReady(t *testing.T) {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: metav1.NamespaceDefault,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	clientSet1 := fakeclient.NewClientBuilder().Build()
	clientSet2 := fakeclient.NewClientBuilder().WithObjects(pod).Build()

	tests := []struct {
		name       string
		kubeClient client.Client
		pod        *corev1.Pod
		want       reconcile.Result
		wantErr    bool
	}{
		{
			name:       "pod1",
			kubeClient: clientSet1,
			pod:        pod,
			want:       reconcile.Result{},
			wantErr:    false,
		},
		{
			name:       "pod2",
			kubeClient: clientSet2,
			pod:        pod,
			want:       reconcile.Result{Requeue: true},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "pod2" {
				newPod := tt.pod.DeepCopy()
				newPod.Status.PodIP = "1.1.1.1"
				tt.kubeClient.Status().Update(context.TODO(), newPod)
			}

			got, err := MarkPodReady(tt.kubeClient, tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarkPodReady() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MarkPodReady() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodePoolCreate(t *testing.T) {
	tests := []struct {
		name  string
		event event.CreateEvent
		want  bool
	}{
		{
			name: "test1",
			event: event.CreateEvent{
				Object: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool1",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   1,
						UnreadyNodeNum: 0,
						Nodes:          TestNodesName,
					},
				},
			},
			want: false,
		},
		{
			name: "test2",
			event: event.CreateEvent{
				Object: &appsv1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nodePool2",
					},
					Status: appsv1beta1.NodePoolStatus{
						ReadyNodeNum:   0,
						UnreadyNodeNum: 1,
						Nodes:          TestNodesName,
					},
				},
			},
			want: true,
		},
		{
			name: "test3",
			event: event.CreateEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodePoolCreate(tt.event); got != tt.want {
				t.Errorf("nodePoolCreate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodUpdate(t *testing.T) {
	tests := []struct {
		name  string
		event event.UpdateEvent
		want  bool
	}{
		{
			name: "test1",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
				ObjectNew: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						}},
					},
				},
			},
			want: true,
		}, {
			name: "test2",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
				ObjectNew: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			want: false,
		}, {
			name: "test3",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			want: false,
		}, {
			name: "test4",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},

				ObjectNew: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: metav1.NamespaceDefault,
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := podUpdate(tt.event); got != tt.want {
				t.Errorf("podUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
