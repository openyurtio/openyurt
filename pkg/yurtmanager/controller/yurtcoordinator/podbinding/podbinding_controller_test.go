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

package podbinding

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

var (
	TestNodesName = []string{"node1", "node2", "node3", "node4"}
	TestPodsName  = []string{"pod1", "pod2", "pod3", "pod4"}
)

func prepareNodes() []client.Object {
	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Annotations: map[string]string{
					"node.beta.openyurt.io/autonomy": "true",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
				Annotations: map[string]string{
					"apps.openyurt.io/binding": "true",
				},
			},
		},
	}
	return nodes
}

func preparePods() []client.Object {
	second1 := int64(300)
	second2 := int64(100)
	pods := []client.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: metav1.NamespaceDefault,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "DaemonSet",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: metav1.NamespaceDefault,
				Annotations: map[string]string{
					corev1.MirrorPodAnnotationKey: "03b446125f489d8b04a90de0899657ca",
				},
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:      corev1.TaintNodeNotReady,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
				},
				NodeName: "node1",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:               corev1.TaintNodeNotReady,
						Operator:          corev1.TolerationOpExists,
						Effect:            corev1.TaintEffectNoExecute,
						TolerationSeconds: &second1,
					},
					{
						Key:               corev1.TaintNodeUnreachable,
						Operator:          corev1.TolerationOpExists,
						Effect:            corev1.TaintEffectNoExecute,
						TolerationSeconds: &second1,
					},
				},
				NodeName: "node1",
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod4",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:               corev1.TaintNodeNotReady,
						Operator:          corev1.TolerationOpExists,
						Effect:            corev1.TaintEffectNoExecute,
						TolerationSeconds: &second2,
					},
				},
			},
		},
	}

	return pods
}

func TestReconcile(t *testing.T) {
	pods := preparePods()
	nodes := prepareNodes()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}
	c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(pods...).WithObjects(nodes...).Build()

	for i := range TestNodesName {
		var req = reconcile.Request{NamespacedName: types.NamespacedName{Name: TestNodesName[i]}}
		rsp := ReconcilePodBinding{
			Client: c,
		}

		_, err := rsp.Reconcile(context.TODO(), req)
		if err != nil {
			t.Errorf("Reconcile() error = %v", err)
			return
		}

		pod := &corev1.Pod{}
		err = c.Get(context.TODO(), types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: TestPodsName[i]}, pod)
		if err != nil {
			continue
		}
		t.Logf("pod %s Tolerations is %+v", TestPodsName[i], pod.Spec.Tolerations)
	}
}

func TestConfigureTolerationForPod(t *testing.T) {
	pods := preparePods()
	nodes := prepareNodes()
	c := fakeclient.NewClientBuilder().WithObjects(pods...).WithObjects(nodes...).Build()

	second := int64(300)
	tests := []struct {
		name              string
		pod               *corev1.Pod
		tolerationSeconds *int64
		wantErr           bool
	}{
		{
			name:              "test1",
			pod:               pods[0].(*corev1.Pod),
			tolerationSeconds: &second,
			wantErr:           false,
		},
		{
			name:              "test2",
			pod:               pods[1].(*corev1.Pod),
			tolerationSeconds: &second,
			wantErr:           false,
		},
		{
			name:              "test3",
			pod:               pods[2].(*corev1.Pod),
			tolerationSeconds: &second,
			wantErr:           false,
		},
		{
			name:              "test4",
			pod:               pods[3].(*corev1.Pod),
			tolerationSeconds: &second,
			wantErr:           false,
		},
		{
			name: "test5",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod5",
					Namespace: metav1.NamespaceDefault,
				},
			},
			tolerationSeconds: &second,
			wantErr:           true,
		},
		{
			name: "test6",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod5",
					Namespace: metav1.NamespaceDefault,
				},
			},
			tolerationSeconds: nil,
			wantErr:           true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcilePodBinding{
				Client: c,
			}
			if err := r.configureTolerationForPod(tt.pod, tt.tolerationSeconds); (err != nil) != tt.wantErr {
				t.Errorf("configureTolerationForPod() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetPodsAssignedToNode(t *testing.T) {
	pods := preparePods()
	c := fakeclient.NewClientBuilder().WithObjects(pods...).Build()
	tests := []struct {
		name     string
		nodeName string
		want     []corev1.Pod
		wantErr  bool
	}{
		{
			name:     "test1",
			nodeName: "node1",
			want: []corev1.Pod{
				*pods[0].(*corev1.Pod),
				*pods[1].(*corev1.Pod),
				*pods[2].(*corev1.Pod),
				*pods[3].(*corev1.Pod),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcilePodBinding{
				Client: c,
			}
			// By the way, the fake client not support ListOptions.FieldSelector, only Namespace and LabelSelector
			// For more details, see sigs.k8s.io/controller-runtime@v0.10.3/pkg/client/fake/client.go:366
			got, err := r.getPodsAssignedToNode(tt.nodeName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPodsAssignedToNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPodsAssignedToNode() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddOrUpdateTolerationInPodSpec(t *testing.T) {
	pods := preparePods()
	second := int64(300)
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "toleration1",
			pod:  pods[0].(*corev1.Pod),
			want: true,
		},
		{
			name: "toleration2",
			pod:  pods[1].(*corev1.Pod),
			want: false,
		},
		{
			name: "toleration3",
			pod:  pods[2].(*corev1.Pod),
			want: false,
		},
		{
			name: "toleration4",
			pod:  pods[3].(*corev1.Pod),
			want: true,
		},
	}
	for _, tt := range tests {
		toleration := corev1.Toleration{
			Key:               corev1.TaintNodeNotReady,
			Operator:          corev1.TolerationOpExists,
			Effect:            corev1.TaintEffectNoExecute,
			TolerationSeconds: &second,
		}
		if tt.name == "toleration2" {
			toleration = corev1.Toleration{
				Key:      corev1.TaintNodeNotReady,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoExecute,
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			if got := addOrUpdateTolerationInPodSpec(&tt.pod.Spec, &toleration); got != tt.want {
				t.Errorf("addOrUpdateTolerationInPodSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDaemonSetPodOrStaticPod(t *testing.T) {
	pods := preparePods()
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod0",
			pod:  nil,
			want: false,
		},
		{
			name: "pod1",
			pod:  pods[0].(*corev1.Pod),
			want: true,
		},
		{
			name: "pod2",
			pod:  pods[1].(*corev1.Pod),
			want: true,
		},
		{
			name: "pod3",
			pod:  pods[2].(*corev1.Pod),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDaemonSetPodOrStaticPod(tt.pod); got != tt.want {
				t.Errorf("isDaemonSetPodOrStaticPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPodBoundenToNode(t *testing.T) {
	nodes := prepareNodes()
	tests := []struct {
		name string
		node *corev1.Node
		want bool
	}{
		{
			name: "node1",
			node: nodes[0].(*corev1.Node),
			want: false,
		},
		{
			name: "node2",
			node: nodes[1].(*corev1.Node),
			want: true,
		},
		{
			name: "node3",
			node: nodes[2].(*corev1.Node),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeutil.IsPodBoundenToNode(tt.node); got != tt.want {
				t.Errorf("IsPodBoundenToNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newReconciler(t *testing.T) {
	tests := []struct {
		name string
		in0  *appconfig.CompletedConfig
		mgr  manager.Manager
		want reconcile.Reconciler
	}{
		{
			name: "test1",
			in0:  nil,
			mgr:  nil,
			want: &ReconcilePodBinding{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newReconciler(tt.in0, tt.mgr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newReconciler() = %v, want %v", got, tt.want)
			}
		})
	}
}
