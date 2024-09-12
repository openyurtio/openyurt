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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestYurtStaticSetHandler_ValidateCreate(t *testing.T) {
	tests := []struct {
		name        string
		obj         runtime.Object
		expectError bool
		errorMsg    string
	}{
		{
			name:        "should fail when obj type is not StaticPodManifest",
			obj:         &runtime.Unknown{},
			expectError: true,
			errorMsg:    "expected a YurtStaticSet but got a *runtime.Unknown",
		},
		{
			name: "should fail when StaticPodManifest is empty",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{},
			},
			expectError: true,
			errorMsg:    "StaticPodManifest is required",
		},
		{
			name: "should fail when unsupported upgrade strategy type is used",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "manifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: "InvalidStrategyType",
					},
				},
			},
			expectError: true,
			errorMsg:    "supported values: \"OTA\", \"AdvancedRollingUpdate\"",
		},
		{
			name: "should fail when MaxUnavailable is nil in AdvancedRollingUpdate mode",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "manifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: v1alpha1.AdvancedRollingUpdateUpgradeStrategyType,
					},
				},
			},
			expectError: true,
			errorMsg:    "max-unavailable is required in AdvancedRollingUpdate mode",
		},
		{
			name: "should pass when YurtStaticSet is valid",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "manifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: v1alpha1.OTAUpgradeStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: buildValidPod().ObjectMeta,
						Spec:       buildValidPod().Spec,
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &YurtStaticSetHandler{}
			_, err := handler.ValidateCreate(context.Background(), tt.obj)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestYurtStaticSetHandler_ValidateUpdate(t *testing.T) {
	tests := []struct {
		name        string
		oldObj      runtime.Object
		newObj      runtime.Object
		expectError bool
		errorMsg    string
	}{
		{
			name:   "should fail when updating old obj is not YurtStaticSet",
			oldObj: &runtime.Unknown{},
			newObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "newManifest",
				},
			},
			expectError: true,
			errorMsg:    "expected a YurtStaticSet but got a *runtime.Unknown",
		},
		{
			name: "should fail when updating new obj is not YurtStaticSet",
			oldObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "oldManifest",
				},
			},
			newObj:      &runtime.Unknown{},
			expectError: true,
			errorMsg:    "expected a YurtStaticSet but got a *runtime.Unknown",
		},
		{
			name: "should fail when new YurtStaticSet has invalid StaticPodManifest",
			oldObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "oldManifest",
				},
			},
			newObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{},
			},
			expectError: true,
			errorMsg:    "StaticPodManifest is required",
		},
		{
			name: "should fail when old YurtStaticSet has invalid StaticPodManifest",
			newObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "newManifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: v1alpha1.OTAUpgradeStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: buildValidPod().ObjectMeta,
						Spec:       buildValidPod().Spec,
					},
				},
			},
			oldObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{},
			},
			expectError: true,
			errorMsg:    "StaticPodManifest is required",
		},
		{
			name: "should pass when updating YurtStaticSet with valid changes",
			oldObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "oldManifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: v1alpha1.OTAUpgradeStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: buildValidPod().ObjectMeta,
						Spec:       buildValidPod().Spec,
					},
				},
			},
			newObj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{
					StaticPodManifest: "newManifest",
					UpgradeStrategy: v1alpha1.YurtStaticSetUpgradeStrategy{
						Type: v1alpha1.OTAUpgradeStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: buildValidPod().ObjectMeta,
						Spec:       buildValidPod().Spec,
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &YurtStaticSetHandler{}
			_, err := handler.ValidateUpdate(context.Background(), tt.oldObj, tt.newObj)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestYurtStaticSetHandler_ValidateDelete(t *testing.T) {
	tests := []struct {
		name        string
		obj         runtime.Object
		expectError bool
		errorMsg    string
	}{
		{
			name: "should pass when deleting a YurtStaticSet",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &YurtStaticSetHandler{}
			_, err := handler.ValidateDelete(context.Background(), tt.obj)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func buildValidPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "example",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx-container",
					Image: "nginx:1.19.2",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "ENV_VAR_EXAMPLE",
							Value: "value",
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("128m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("128m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config-volume",
							MountPath: "/etc/config",
						},
					},
					TerminationMessagePolicy: corev1.TerminationMessageReadFile, // 设置 terminationMessagePolicy
					ImagePullPolicy:          corev1.PullIfNotPresent,           // 设置 imagePullPolicy
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "config-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "example-config",
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
			DNSPolicy:     corev1.DNSClusterFirst, // 设置 dnsPolicy
			NodeSelector: map[string]string{
				"disktype": "ssd",
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      "key1",
					Operator: corev1.TolerationOpEqual,
					Value:    "value1",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/e2e-az-name",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"e2e-az1", "e2e-az2"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
