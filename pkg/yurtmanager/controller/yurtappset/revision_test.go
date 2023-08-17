/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.
Copyright 2017 The Kubernetes Authors.

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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclint "sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestReconcileyurtAppSet_ControlledHistories(t *testing.T) {

	instance := struct {
		yas *appsv1alpha1.YurtAppSet
		ctr []*appsv1.ControllerRevision
	}{
		yas: &appsv1alpha1.YurtAppSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: appsv1alpha1.YurtAppSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"name": "foo",
					},
				},
				WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"name": "foo",
							},
						},
						Spec: appsv1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"name": "foo",
									},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "container-a",
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
				RevisionHistoryLimit: &two,
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
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add appsv1 custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance.yas).Build()
	ryas := ReconcileYurtAppSet{
		Client: fc,
		scheme: scheme,
	}
	histories, err := ryas.controlledHistories(instance.yas)
	if err != nil || len(histories) != 0 {
		t.Logf("failed to get controlled histories")
	}

}

func TestReconcileYurtAppSet_CreateControllerRevision(t *testing.T) {
	instance := struct {
		yas *appsv1alpha1.YurtAppSet
		ctr *appsv1.ControllerRevision
	}{
		yas: &appsv1alpha1.YurtAppSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: appsv1alpha1.YurtAppSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"name": "foo",
					},
				},
				WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"name": "foo",
							},
						},
						Spec: appsv1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"name": "foo",
									},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "container-a",
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
							Name: "pool-a",
							NodeSelectorTerm: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-name",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"nodeA"},
									},
								},
							},
						},
					},
				},
				RevisionHistoryLimit: &two,
			},
		},
		ctr: &appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-0",
				Namespace: "foo-ns",
			},
			Revision: 1,
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
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add appsv1 custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance.yas).Build()
	ryas := ReconcileYurtAppSet{
		Client: fc,
		scheme: scheme,
	}
	o, err := ryas.createControllerRevision(instance.yas, instance.ctr, &two)
	if err != nil && o.Revision != 1 {
		t.Logf("failed to create controller revision")
	}
}

func TestReconcileYurtAppSet_newRevision(t *testing.T) {
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
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.StatefulSetSpec{
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
										Name:  "nginx",
										Image: "nginx:1.19",
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
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add appsv1 custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	ryas := ReconcileYurtAppSet{
		Client: fc,
		scheme: scheme,
	}
	cr, err := ryas.newRevision(instance, 2, &two)
	if err != nil && cr.Namespace != instance.Namespace {
		t.Logf("failed to new revision for yurtappset")
	}
}

func TestReconcileYurtAppSet_ConstructYurtAppSetRevisions(t *testing.T) {
	instance := struct {
		yas *appsv1alpha1.YurtAppSet
		ctr *appsv1.ControllerRevision
	}{
		yas: &appsv1alpha1.YurtAppSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "foo-ns",
			},
			Spec: appsv1alpha1.YurtAppSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"name": "foo",
					},
				},
				WorkloadTemplate: appsv1alpha1.WorkloadTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"name": "foo",
							},
						},
						Spec: appsv1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"name": "foo",
									},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "container-a",
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
							Name: "pool-a",
							NodeSelectorTerm: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-name",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"nodeA"},
									},
								},
							},
						},
					},
				},
				RevisionHistoryLimit: &two,
			},
		},
		ctr: &appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-0",
				Namespace: "foo-ns",
			},
			Revision: 1,
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
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add appsv1 custom resource")
		return
	}

	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(instance.yas).Build()
	ryas := ReconcileYurtAppSet{
		Client: fc,
		scheme: scheme,
	}
	_, _, _, err := ryas.constructYurtAppSetRevisions(instance.yas)
	if err != nil {
		t.Logf("failed to construct yurtappset revisions")
	}
}
