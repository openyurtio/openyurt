/*
Copyright 2024 The OpenYurt Authors.

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

package workloadmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/scale/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

var (
	itemReplicas int32 = 3
	fakeScheme         = newOpenYurtScheme()
)

func newOpenYurtScheme() *runtime.Scheme {
	myScheme := runtime.NewScheme()

	apis.AddToScheme(myScheme)
	scheme.AddToScheme(myScheme)
	appsv1.AddToScheme(myScheme)

	return myScheme
}

var testDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
	},
	Spec: appsv1.DeploymentSpec{
		Replicas: &itemReplicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:  "initContainer",
						Image: "initOld",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "configMapSource",
								},
							},
						},
					},
				},
			},
		},
	},
}

func TestGetNodePoolTweaksFromYurtAppSet(t *testing.T) {
	type args struct {
		cli          client.Client
		nodepoolName string
		yas          *v1beta1.YurtAppSet
	}
	tests := []struct {
		name    string
		args    args
		want    []*v1beta1.Tweaks
		wantErr bool
	}{
		// Test case 1
		{
			name: "nodepool matches yurtappset",
			args: args{
				cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
					&v1beta1.NodePool{ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool",
					}},
					&v1beta1.YurtAppSet{
						Spec: v1beta1.YurtAppSetSpec{
							Workload: v1beta1.Workload{
								WorkloadTweaks: []v1beta1.WorkloadTweak{
									{
										Pools: []string{"test-nodepool"},
										Tweaks: v1beta1.Tweaks{
											Replicas: &itemReplicas,
										},
									},
								},
							},
						}},
				).Build(),
				nodepoolName: "test-nodepool",
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Workload: v1beta1.Workload{
							WorkloadTweaks: []v1beta1.WorkloadTweak{
								{
									Pools: []string{"test-nodepool"},
									Tweaks: v1beta1.Tweaks{
										Replicas: &itemReplicas,
									},
								},
							},
						},
					},
				},
			},
			want: []*v1beta1.Tweaks{
				{Replicas: &itemReplicas},
			},
			wantErr: false,
		},
		// Test case 2
		{
			name: "no nodepool selector or pools specified",
			args: args{
				cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
					&v1beta1.NodePool{ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool",
					}},
					&v1beta1.YurtAppSet{
						Spec: v1beta1.YurtAppSetSpec{},
					},
				).Build(),
				nodepoolName: "test-nodepool",
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{},
				},
			},
			want:    []*v1beta1.Tweaks{},
			wantErr: false,
		},
		// Test case 3
		{
			name: "nodepool selector match",
			args: args{
				cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
					&v1beta1.NodePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-nodepool",
							Labels: map[string]string{
								"app": "test-nodepool",
							},
						},
					},
					&v1beta1.YurtAppSet{
						Spec: v1beta1.YurtAppSetSpec{
							Workload: v1beta1.Workload{
								WorkloadTweaks: []v1beta1.WorkloadTweak{
									{
										NodePoolSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "test-nodepool",
											},
										},
										Tweaks: v1beta1.Tweaks{
											Replicas: &itemReplicas,
										},
									},
								},
							},
						},
					},
				).Build(),
				nodepoolName: "test-nodepool",
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Workload: v1beta1.Workload{
							WorkloadTweaks: []v1beta1.WorkloadTweak{
								{
									NodePoolSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "test-nodepool",
										},
									},
									Tweaks: v1beta1.Tweaks{
										Replicas: &itemReplicas,
									},
								},
							},
						},
					},
				},
			},
			want: []*v1beta1.Tweaks{
				{Replicas: &itemReplicas},
			},
			wantErr: false,
		},
		// Test case 4
		{
			name: "no nodepools",
			args: args{
				cli: fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
					&v1beta1.YurtAppSet{
						Spec: v1beta1.YurtAppSetSpec{},
					},
				).Build(),
				nodepoolName: "test-nodepool",
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{},
				},
			},
			want:    []*v1beta1.Tweaks{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetNodePoolTweaksFromYurtAppSet(tt.args.cli, tt.args.nodepoolName, tt.args.yas)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodePoolTweaksFromYurtAppSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.want, got) {
				t.Errorf("GetNodePoolTweaksFromYurtAppSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyBasicTweaksToDeployment(t *testing.T) {
	var targetReplicas int32 = 2
	items := []*v1beta1.Tweaks{
		{
			ContainerImages: []v1beta1.ContainerImage{
				{
					Name:        "nginx",
					TargetImage: "nginx-test",
				},
				{
					Name:        "initContainer",
					TargetImage: "initNew",
				},
			},
			Replicas: &targetReplicas,
		},
	}
	applyBasicTweaksToDeployment(testDeployment, items)
	assert.Equal(t, "nginx-test", testDeployment.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, "initNew", testDeployment.Spec.Template.Spec.InitContainers[0].Image)
	assert.Equal(t, targetReplicas, *testDeployment.Spec.Replicas)
}

func TestApplyAdavancedTweaksToDeployment(t *testing.T) {
	patches := []v1beta1.Patch{
		{
			Operation: v1beta1.REPLACE,
			Path:      "/spec/template/spec/containers/0/image",
			Value: apiextensionsv1.JSON{
				Raw: []byte(`"tomcat:1.18"`),
			},
		},
		{
			Operation: v1beta1.ADD,
			Path:      "/spec/replicas",
			Value: apiextensionsv1.JSON{
				Raw: []byte("5"),
			},
		},
	}

	var initialReplicas int32 = 2
	var testPatchDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps.openyurt.io/v1alpha1",
				Kind:       "YurtAppSet",
				Name:       "yurtappset-patch",
			}},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &initialReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
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
	}

	// apply no patches
	testPatchDeploymentCopy := testPatchDeployment.DeepCopy()
	applyAdvancedTweaksToDeployment(testPatchDeploymentCopy, []*v1beta1.Tweaks{})
	assert.Equal(t, testPatchDeploymentCopy, testPatchDeployment)

	// apply patches
	applyAdvancedTweaksToDeployment(testPatchDeployment, []*v1beta1.Tweaks{
		{Patches: patches},
	})
	assert.Equal(t, "tomcat:1.18", testPatchDeployment.Spec.Template.Spec.Containers[0].Image)
}

func TestApplyTweaksToDeployment(t *testing.T) {
	type args struct {
		deployment *appsv1.Deployment
		tweaks     []*v1beta1.Tweaks
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Test case 1: BasicTweaks is not empty, AdvancedTweaks is empty.",
			args: args{
				deployment: &appsv1.Deployment{},
				tweaks: []*v1beta1.Tweaks{
					{
						ContainerImages: []v1beta1.ContainerImage{
							{
								Name:        "nginx",
								TargetImage: "nginx-test",
							},
							{
								Name:        "initContainer",
								TargetImage: "initNew",
							},
						},
					},
					{
						Replicas: &itemReplicas,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Test case 2: Both BasicTweaks and AdvancedTweaks are not empty.",
			args: args{
				deployment: &appsv1.Deployment{},
				tweaks: []*v1beta1.Tweaks{
					{
						ContainerImages: []v1beta1.ContainerImage{
							{
								Name:        "nginx",
								TargetImage: "nginx-test",
							},
							{
								Name:        "initContainer",
								TargetImage: "initNew",
							},
						},
					},
					{
						Replicas: &itemReplicas,
					},
					{
						Patches: []v1beta1.Patch{
							{
								Operation: v1beta1.REPLACE,
								Path:      "/spec/template/spec/containers/0/image",
								Value: apiextensionsv1.JSON{
									Raw: []byte(`"tomcat:1.18"`),
								},
							},
							{
								Operation: v1beta1.ADD,
								Path:      "/spec/replicas",
								Value: apiextensionsv1.JSON{
									Raw: []byte("5"),
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Test case 3: Both BasicTweaks and AdvancedTweaks are empty.",
			args: args{
				deployment: &appsv1.Deployment{},
				tweaks:     []*v1beta1.Tweaks{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyTweaksToDeployment(tt.args.deployment, tt.args.tweaks)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyTweaksToDeployment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
