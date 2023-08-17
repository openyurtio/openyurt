/*
Copyright 2022 The OpenYurt Authors.

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

package adapter

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclint "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestStatefulSetAdapter_ApplyPoolTemplate(t *testing.T) {
	var one int32 = 1
	cases := []struct {
		name     string
		yas      *appsv1alpha1.YurtAppSet
		poolName string
		revision string
		replicas int32
		obj      runtime.Object
		wantSts  *appsv1.StatefulSet
	}{
		{
			name: "apply pool template",
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
						StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									appsv1alpha1.AnnotationPatchKey: "annotation-v",
								},
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: appsv1.StatefulSetSpec{
								Replicas: &one,
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
								Name: "hangzhou",
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
					RevisionHistoryLimit: &one,
				},
			},
			poolName: "hangzhou",
			revision: "1",
			replicas: one,
			obj:      &appsv1.StatefulSet{},

			wantSts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels: map[string]string{
						"name": "foo",
						appsv1alpha1.ControllerRevisionHashLabelKey: "1",
						appsv1alpha1.PoolNameLabelKey:               "hangzhou",
					},
					Annotations: map[string]string{
						appsv1alpha1.AnnotationPatchKey: "",
					},
					GenerateName: "foo-hangzhou-",
				},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name":                        "foo",
							appsv1alpha1.PoolNameLabelKey: "hangzhou",
						},
					},
					Replicas: &one,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"name": "foo",
								appsv1alpha1.ControllerRevisionHashLabelKey: "1",
								appsv1alpha1.PoolNameLabelKey:               "hangzhou",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "container-a",
									Image: "nginx:1.0",
								},
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
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
							},
						},
					},
					RevisionHistoryLimit: &one,
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
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	sa := StatefulSetAdapter{Client: fc, Scheme: scheme}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := sa.ApplyPoolTemplate(tt.yas, tt.poolName, tt.revision, tt.replicas, tt.obj)
			if err != nil {
				t.Logf("failed to appply pool template")
			}
			if err = controllerutil.SetControllerReference(tt.yas, tt.wantSts, sa.Scheme); err != nil {
				panic(err)
			}
		})
	}
}

func TestStatefulSetAdapter_GetDetails(t *testing.T) {
	var one int32 = 1
	tests := []struct {
		name             string
		obj              metav1.Object
		wantReplicasInfo ReplicasInfo
	}{
		{
			name: "get statefulsetAdapter details",
			obj: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &one,
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: one,
				},
			},
			wantReplicasInfo: ReplicasInfo{
				Replicas:      one,
				ReadyReplicas: one,
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
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	sa := StatefulSetAdapter{Client: fc, Scheme: scheme}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := sa.GetDetails(tt.obj)
			if err != nil || got.Replicas != tt.wantReplicasInfo.Replicas {
				t.Logf("failed to get details")
			}
		})
	}
}
