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
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestYurtAppDaemonHandler_ValidateCreate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		obj         runtime.Object
		expectErr   bool
		expectedMsg string
	}{
		{
			name:        "should return error when object is not YurtAppDaemon",
			obj:         &runtime.Unknown{},
			expectErr:   true,
			expectedMsg: "expected a YurtAppDaemon but got a *runtime.Unknown",
		},
		{
			name:        "should return error when YurtAppDaemon is invalid",
			obj:         mockInvalidYurtAppDaemon(),
			expectErr:   true,
			expectedMsg: "metadata.name: Required value: name or generateName is required",
		},
		{
			name:        "should return error when YurtAppDaemon With Invalid Deploy",
			obj:         mockValidYurtAppDaemonWithInvalidDeploy(),
			expectErr:   true,
			expectedMsg: "spec.template.deploymentTemplate.spec.template.spec.containers",
		},
		{
			name:      "should succeed when YurtAppDaemon is valid",
			obj:       mockValidYurtAppDaemonWithStatefulSet(),
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &YurtAppDaemonHandler{}
			_, err := handler.ValidateCreate(context.TODO(), tc.obj)
			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestYurtAppDaemonHandler_ValidateUpdate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		oldObj         runtime.Object
		newObj         runtime.Object
		expectErr      bool
		expectedErrMsg string
	}{
		{
			name:           "should return error when oldObj is not YurtAppDaemon",
			oldObj:         &runtime.Unknown{},
			newObj:         mockValidYurtAppDaemonWithStatefulSet(),
			expectErr:      true,
			expectedErrMsg: "expected a YurtAppDaemon but got a *runtime.Unknown",
		},
		{
			name:           "should return error when newObj is not YurtAppDaemon",
			oldObj:         mockValidYurtAppDaemonWithStatefulSet(),
			newObj:         &runtime.Unknown{},
			expectErr:      true,
			expectedErrMsg: "expected a YurtAppDaemon but got a *runtime.Unknown",
		},
		{
			name:           "should return error when YurtAppDaemon update is invalid",
			oldObj:         mockValidYurtAppDaemonWithStatefulSet(),
			newObj:         mockInvalidYurtAppDaemon(),
			expectErr:      true,
			expectedErrMsg: "metadata.name: Required value",
		},
		{
			name:   "should succeed when YurtAppDaemonWithStatefulSet update is valid",
			oldObj: mockValidYurtAppDaemonWithStatefulSet(),
			newObj: func() runtime.Object {
				daemon := mockValidYurtAppDaemonWithStatefulSet()
				daemon.SetResourceVersion("v2")
				return daemon
			}(),
			expectErr: false,
		},
		{
			name:   "should succeed when YurtAppDaemonWithDeploy update is valid",
			oldObj: mockValidYurtAppDaemonWithDeploy(),
			newObj: func() runtime.Object {
				daemon := mockValidYurtAppDaemonWithDeploy()
				daemon.SetResourceVersion("v2")
				return daemon
			}(),
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &YurtAppDaemonHandler{}
			_, err := handler.ValidateUpdate(context.TODO(), tc.oldObj, tc.newObj)
			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateDelete(t *testing.T) {
	handler := &YurtAppDaemonHandler{}
	_, err := handler.ValidateDelete(context.TODO(), &v1alpha1.YurtAppDaemon{})
	assert.Nil(t, err)
}

func mockValidYurtAppDaemonWithDeploy() *v1alpha1.YurtAppDaemon {
	return &v1alpha1.YurtAppDaemon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-yurtappdaemon",
			Namespace: "default",
		},
		Spec: v1alpha1.YurtAppDaemonSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "yurtappdaemon"},
			},
			WorkloadTemplate: v1alpha1.WorkloadTemplate{

				DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "yurtappdaemon"},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "yurtappdaemon"},
							},
							Spec: buildValidPod().Spec,
						},
					},
				},
			},
		},
	}
}

func mockValidYurtAppDaemonWithStatefulSet() *v1alpha1.YurtAppDaemon {
	return &v1alpha1.YurtAppDaemon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-yurtappdaemon",
			Namespace: "default",
		},
		Spec: v1alpha1.YurtAppDaemonSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "yurtappdaemon"},
			},
			WorkloadTemplate: v1alpha1.WorkloadTemplate{

				StatefulSetTemplate: &v1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "yurtappdaemon"},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "yurtappdaemon"},
							},
						},
					},
				},
			},
		},
	}
}

func mockValidYurtAppDaemonWithInvalidDeploy() *v1alpha1.YurtAppDaemon {
	yurtApp := mockValidYurtAppDaemonWithStatefulSet()
	newApp := yurtApp.DeepCopy()
	newApp.Spec.WorkloadTemplate.StatefulSetTemplate = nil
	newApp.Spec.WorkloadTemplate.DeploymentTemplate = &v1alpha1.DeploymentTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": "yurtappdaemon",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "yurtappdaemon",
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
	}
	return newApp
}

func mockInvalidYurtAppDaemon() *v1alpha1.YurtAppDaemon {
	return &v1alpha1.YurtAppDaemon{
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       v1alpha1.YurtAppDaemonSpec{},
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
			TerminationGracePeriodSeconds: ptr.To[int64](30),
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
