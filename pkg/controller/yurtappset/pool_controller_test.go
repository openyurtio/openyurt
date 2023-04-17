/*
Copyright 2022 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

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

package yurtappset

import (
	"strconv"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclint "sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	adpt "github.com/openyurtio/openyurt/pkg/controller/yurtappset/adapter"
)

var (
	one int32 = 1
	two int32 = 2
)

func TestPoolControl_GetAllPools(t *testing.T) {

	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "foo-ns",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "foo",
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &two,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name:     "foo-0",
						Replicas: &one,
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "app.openyurt.io/nodepool",
									Operator: corev1.NodeSelectorOpIn,
									Values: []string{
										"foo-0",
									},
								},
							},
						},
					},
					{
						Name:     "foo-1",
						Replicas: &two,
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "app.openyurt.io/nodepool",
									Operator: corev1.NodeSelectorOpIn,
									Values: []string{
										"foo-1",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).Build()
	pc := PoolControl{
		Client:  fc,
		scheme:  scheme,
		adapter: &adpt.DeploymentAdapter{Client: fc, Scheme: scheme},
	}
	for i := 0; i < 2; i++ {
		tf := pc.CreatePool(instance, "foo-"+strconv.FormatInt(int64(i), 10), "v0.1.0", two)
		if tf != nil {
			t.Logf("failed create node pool resource")
		}
	}
	pools, err := pc.GetAllPools(instance)
	if err != nil && len(pools) != 2 {
		t.Logf("failed to get the pools of yurtappset")
	}
}

func TestPoolControl_UpdatePool(t *testing.T) {

	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "foo-ns",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "foo",
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &two,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name:     "foo",
						Replicas: &one,
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "app.openyurt.io/nodepool",
									Operator: corev1.NodeSelectorOpIn,
									Values: []string{
										"foo-0",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).Build()
	pc := PoolControl{
		Client:  fc,
		scheme:  scheme,
		adapter: &adpt.DeploymentAdapter{Client: fc, Scheme: scheme},
	}
	tf := pc.CreatePool(instance, "foo", "v0.1.0", two)
	if tf != nil {
		t.Logf("failed create node pool resource")
	}
	pools, err := pc.GetAllPools(instance)
	if err != nil && len(pools) != 2 {
		t.Logf("failed to get the pools of yurtappset")
	}
	tf = pc.UpdatePool(pools[0], instance, "v2", one)
	if tf != nil {
		t.Logf("failed update node pool resource")
	}
}

func TestPoolControl_DeletePool(t *testing.T) {
	instance := &appsv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "foo-ns",
		},
		Spec: appsv1alpha1.YurtAppSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "foo",
				},
			},
			WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
				DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &two,
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Pools: []appsv1alpha1.Pool{
					{
						Name:     "foo",
						Replicas: &one,
					},
				},
			},
		},
	}
	scheme := runtime.NewScheme()
	if err := appsv1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance).Build()
	pc := PoolControl{
		Client:  fc,
		scheme:  scheme,
		adapter: &adpt.DeploymentAdapter{Client: fc, Scheme: scheme},
	}
	tf := pc.CreatePool(instance, "foo", "v0.1.0", two)
	if tf != nil {
		t.Logf("failed create node pool resource")
	}
	pools, err := pc.GetAllPools(instance)
	if err != nil && len(pools) != 2 {
		t.Logf("failed to get the pools of yurtappset")
	}
	tf = pc.DeletePool(pools[0])
	if tf != nil {
		t.Logf("failed update node pool resource")
	}
}
