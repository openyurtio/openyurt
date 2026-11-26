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

package yurtstaticset

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
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
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
	var strategy = []appsv1alpha1.YurtStaticSetUpgradeStrategy{
		{Type: appsv1alpha1.OTAUpgradeStrategyType},
		{Type: appsv1alpha1.AdvancedRollingUpdateUpgradeStrategyType, MaxUnavailable: &DefaultMaxUnavailable},
	}
	staticPods := prepareStaticPods()
	nodes := prepareNodes()
	instance := &appsv1alpha1.YurtStaticSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestStaticPodName,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1alpha1.YurtStaticSetSpec{
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
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).WithStatusSubresource(instance).WithObjects(staticPods...).WithObjects(nodes...).Build()

		var req = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: TestStaticPodName}}
		rsp := ReconcileYurtStaticSet{
			Client: c,
			scheme: scheme,
		}

		_, err := rsp.Reconcile(context.TODO(), req)
		if err != nil {
			t.Fatalf("failed to control static-pod controller, %v", err)
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

func TestReconcileYurtStaticSetDeleteConfigMap(t *testing.T) {
	staticPods := prepareStaticPods()
	instance := &appsv1alpha1.YurtStaticSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestStaticPodName,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1alpha1.YurtStaticSetSpec{
			StaticPodManifest: "nginx",
			Template:          corev1.PodTemplateSpec{},
		},
	}
	cmList := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "yurt-static-set-nginx",
				Namespace: metav1.NamespaceDefault,
			},
		},
	}

	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add yurt custom resource")
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}
	c := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).WithObjects(staticPods...).WithObjects(cmList...).Build()

	tests := []struct {
		name      string
		yssName   string
		namespace string
		wantErr   bool
	}{
		{
			name:      "test1",
			yssName:   TestStaticPodName,
			namespace: metav1.NamespaceDefault,
			wantErr:   false,
		},
		{
			name:      "test2",
			yssName:   TestStaticPodName,
			namespace: metav1.NamespaceDefault,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileYurtStaticSet{
				Client: c,
			}
			if err := r.deleteConfigMap(tt.yssName, tt.namespace); (err != nil) != tt.wantErr {
				t.Errorf("deleteConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSyncConfigMapWithNilMaps(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add yurt custom resource")
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes client-go custom resource")
	}

	tests := []struct {
		name        string
		annotations map[string]string
		data        map[string]string
	}{
		{
			name:        "both annotations and data are nil",
			annotations: nil,
			data:        nil,
		},
		{
			name:        "annotations is nil and data is non-nil",
			annotations: nil,
			data:        map[string]string{"dummy": "value"},
		},
		{
			name:        "annotations is non-nil and data is nil",
			annotations: map[string]string{"dummy": "value"},
			data:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &appsv1alpha1.YurtStaticSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-staticset",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: appsv1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "nginx",
					Template:          corev1.PodTemplateSpec{},
				},
			}
			cmName := util.WithConfigMapPrefix(instance.Name)
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        cmName,
					Namespace:   metav1.NamespaceDefault,
					Annotations: tt.annotations,
				},
				Data: tt.data,
			}

			c := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance, cm).Build()
			r := &ReconcileYurtStaticSet{
				Client: c,
			}

			targetHash := "hash123"
			targetData := "manifest-data"
			err := r.syncConfigMap(instance, targetHash, targetData)
			if err != nil {
				t.Fatalf("syncConfigMap() unexpected error = %v", err)
			}

			updatedCM := &corev1.ConfigMap{}
			if err := c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: cmName}, updatedCM); err != nil {
				t.Fatalf("failed to get updated configmap: %v", err)
			}

			if updatedCM.Annotations[StaticPodHashAnnotation] != targetHash {
				t.Errorf("expected annotation %s = %s, got %s", StaticPodHashAnnotation, targetHash, updatedCM.Annotations[StaticPodHashAnnotation])
			}
			if updatedCM.Data[instance.Spec.StaticPodManifest] != targetData {
				t.Errorf("expected data %s = %s, got %s", instance.Spec.StaticPodManifest, targetData, updatedCM.Data[instance.Spec.StaticPodManifest])
			}
		})
	}
}
