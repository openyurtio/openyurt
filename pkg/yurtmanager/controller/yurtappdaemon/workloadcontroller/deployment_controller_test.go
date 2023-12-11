/*
Copyright 2021 The OpenYurt Authors.

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

package workloadcontroller

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclint "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestGetTemplateType(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name   string
		expect v1alpha1.TemplateType
	}{
		{
			"normal",
			v1alpha1.DeploymentTemplateType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := dc.GetTemplateType()
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestApplyTemplate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name     string
		scheme   *runtime.Scheme
		yad      *v1alpha1.YurtAppDaemon
		nodepool v1alpha1.NodePool
		revision string
		set      *appsv1.Deployment
		expect   error
	}{
		{
			name:   "normal",
			scheme: scheme,
			yad: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yad",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"a": "a",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Volumes: []v1.Volume{},
										Containers: []v1.Container{
											{
												VolumeMounts: []v1.VolumeMount{},
											},
										},
									},
								},
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										apps.PoolNameLabelKey: "a",
									},
								},
							},
						},
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"a": "a",
						},
					},
				},
			},
			nodepool: v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "np",
				},
			},
			revision: "1",
			set: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "coredns",
					Annotations: map[string]string{
						"a": "a",
					},
				},
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := dc.ApplyTemplate(tt.scheme, tt.yad, tt.nodepool, tt.revision, tt.set)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestObjectKey(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name   string
		load   *Workload
		expect client.ObjectKey
	}{
		{
			"normal",
			&Workload{
				Name:      "a",
				Namespace: "a",
			},
			types.NamespacedName{
				Namespace: "a",
				Name:      "a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := dc.ObjectKey(tt.load)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestDeploymentControllor_CreateWorkload(t *testing.T) {
	var four int32 = 4
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name     string
		yad      *v1alpha1.YurtAppDaemon
		nodepool v1alpha1.NodePool
		expect   error
	}{
		{
			name: "normal",
			yad: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "foo-ns",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "daemon-foo",
						},
					},
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "daemon-foo",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &four,
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "daemon-foo",
									},
								},
								Template: v1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											"app": "daemon-foo",
										},
									},
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image:           "nginx:1.19",
												ImagePullPolicy: "Always",
												Name:            "nginx",
											},
										},
									},
								},
							},
						},
					},
					NodePoolSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			nodepool: v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1alpha1.NodePoolSpec{
					Type: "Edge",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dc.CreateWorkload(tt.yad, tt.nodepool, "1")
			t.Logf("expect: %v, get: %v", tt.expect, err)
			if !reflect.DeepEqual(err, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, err)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, err)
		})
	}
}

func TestDeploymentControllor_GetAllWorkloads(t *testing.T) {
	var four int32 = 4
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name     string
		yad      *v1alpha1.YurtAppDaemon
		nodepool v1alpha1.NodePool
		expect   error
	}{
		{
			name: "normal",
			yad: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "foo-ns",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "daemon-foo",
						},
					},
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "daemon-foo",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &four,
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "daemon-foo",
									},
								},
								Template: v1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											"app": "daemon-foo",
										},
									},
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image:           "nginx:1.19",
												ImagePullPolicy: "Always",
												Name:            "nginx",
											},
										},
									},
								},
							},
						},
					},
					NodePoolSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			nodepool: v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1alpha1.NodePoolSpec{
					Type: "Edge",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dc.CreateWorkload(tt.yad, tt.nodepool, "1")
			if err != nil {
				t.Logf("sucessed to create a yurtappdaemon")
			}
			ws, err := dc.GetAllWorkloads(tt.yad)
			if err != nil {
				t.Logf("sucessed to get all yurtappdaemon")
			}
			t.Logf("expect: %v, get: %v", tt.expect, err)
			if len(ws) != 0 && !reflect.DeepEqual(err, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, err)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, err)
		})
	}
}

func TestDeploymentControllor_DeleteWorkload(t *testing.T) {
	var four int32 = 4
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	fc := fakeclint.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	dc := DeploymentControllor{
		Client: fc,
		Scheme: scheme,
	}

	tests := []struct {
		name     string
		yad      *v1alpha1.YurtAppDaemon
		nodepool v1alpha1.NodePool
		expect   error
	}{
		{
			name: "normal",
			yad: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "foo-ns",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "daemon-foo",
						},
					},
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "daemon-foo",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &four,
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "daemon-foo",
									},
								},
								Template: v1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											"app": "daemon-foo",
										},
									},
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image:           "nginx:1.19",
												ImagePullPolicy: "Always",
												Name:            "nginx",
											},
										},
									},
								},
							},
						},
					},
					NodePoolSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			nodepool: v1alpha1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1alpha1.NodePoolSpec{
					Type: "Edge",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"nodepool": "foo",
						},
					},
				},
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dc.CreateWorkload(tt.yad, tt.nodepool, "1")
			if err != nil {
				t.Logf("sucessed to create a yurtappdaemon")
			}
			ws, err := dc.GetAllWorkloads(tt.yad)
			if err != nil {
				t.Logf("sucessed to get all yurtappdaemon")
			}
			err = dc.DeleteWorkload(tt.yad, ws[0])
			t.Logf("expect: %v, get: %v", tt.expect, err)
			if !reflect.DeepEqual(err, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, err)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, err)
		})
	}
}
