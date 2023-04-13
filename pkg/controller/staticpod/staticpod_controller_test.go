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

package staticpod

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclint "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod/util"
)

const (
	TestStaticPodName  = "nginx"
	TestStaticPodImage = "nginx:1.19.1"
)

var (
	DefaultMaxUnavailable = intstr.FromString("10%")
	TestNodes             = []string{"node1", "node2", "node3", "node4"}
)

func prepareStaticPods() []client.Object {
	var pods []client.Object
	for _, node := range TestNodes {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            util.Hyphen(TestStaticPodName, node),
				OwnerReferences: []metav1.OwnerReference{{Kind: "Node"}},
				Namespace:       metav1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  TestStaticPodName,
						Image: TestStaticPodImage,
					},
				},
				NodeName: node,
			},
		}

		pods = append(pods, client.Object(pod))
	}
	return pods
}

func prepareNodes() []client.Object {
	var nodes []client.Object
	for _, node := range TestNodes {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: node},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue}}},
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func TestReconcile(t *testing.T) {
	var strategy = []appsv1alpha1.StaticPodUpgradeStrategy{
		{Type: appsv1alpha1.OTAStaticPodUpgradeStrategyType},
		{Type: appsv1alpha1.AutoStaticPodUpgradeStrategyType, MaxUnavailable: &DefaultMaxUnavailable},
	}
	staticPods := prepareStaticPods()
	nodes := prepareNodes()
	instance := &appsv1alpha1.StaticPod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestStaticPodName,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1alpha1.StaticPodSpec{
			StaticPodManifest: "nginx",
			Template:          corev1.PodTemplateSpec{},
		},
	}

	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add yurt custom resource")
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}

	for _, s := range strategy {
		instance.Spec.UpgradeStrategy = s
		c := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).WithObjects(staticPods...).WithObjects(nodes...).Build()

		var req = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: TestStaticPodName}}
		rsp := ReconcileStaticPod{
			Client: c,
			scheme: scheme,
		}

		_, err := rsp.Reconcile(context.TODO(), req)
		if err != nil {
			t.Fatalf("failed to control static-pod controller")
		}
	}
}

func Test_nodeTurnReady(t *testing.T) {
	evt := event.UpdateEvent{
		ObjectNew: &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		ObjectOld: &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
	}
	t.Run("Test_nodeTurnReady", func(t *testing.T) {
		if got := nodeTurnReady(evt); got != true {
			t.Errorf("nodeTurnReady() = %v, want true", got)
		}
	})
}
