/*
Copyright 2025 The OpenYurt Authors.

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

package imagepreheat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
)

func TestJobFilter(t *testing.T) {
	tests := []struct {
		name     string
		obj      client.Object
		expected bool
	}{
		{
			name: "valid job with correct prefix and pod owner reference",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image-pre-pull-test-job",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Pod",
							Name: "test-pod",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "job with correct prefix but no owner reference",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image-pre-pull-test-job",
				},
			},
			expected: false,
		},
		{
			name: "job with correct prefix but wrong owner reference kind",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image-pre-pull-test-job",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "DaemonSet",
							Name: "test-ds",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "job with wrong prefix but pod owner reference",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrong-prefix-job",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Pod",
							Name: "test-pod",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "job with correct prefix and multiple owner references including pod",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "image-pre-pull-test-job",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "DaemonSet",
							Name: "test-ds",
						},
						{
							Kind: "Pod",
							Name: "test-pod",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "non-job object",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			expected: false,
		},
		{
			name:     "nil object",
			obj:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JobFilter(tt.obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPodFilter(t *testing.T) {
	tests := []struct {
		name     string
		obj      client.Object
		expected bool
	}{
		{
			name: "pod with PodImageReady condition status False",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   daemonsetupgradestrategy.PodImageReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with PodImageReady condition status True",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   daemonsetupgradestrategy.PodImageReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with PodImageReady condition status Unknown",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   daemonsetupgradestrategy.PodImageReady,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod without PodImageReady condition",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with multiple conditions including PodImageReady False",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   daemonsetupgradestrategy.PodImageReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod no owner reference",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   daemonsetupgradestrategy.PodImageReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with empty conditions",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
						},
					},
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{},
				},
			},
			expected: false,
		},
		{
			name: "non-pod object",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
				},
			},
			expected: false,
		},
		{
			name:     "nil object",
			obj:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PodFilter(tt.obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJobFilterEdgeCases tests edge cases for JobFilter
func TestJobFilterEdgeCases(t *testing.T) {
	t.Run("job with empty name", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "Pod",
						Name: "test-pod",
					},
				},
			},
		}
		result := JobFilter(job)
		assert.False(t, result)
	})

	t.Run("job with name that starts with prefix but has additional characters", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "image-pre-pull-extra-text",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "Pod",
						Name: "test-pod",
					},
				},
			},
		}
		result := JobFilter(job)
		assert.True(t, result)
	})

	t.Run("job with name that contains prefix but doesn't start with it", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-image-pre-pull-job",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "Pod",
						Name: "test-pod",
					},
				},
			},
		}
		result := JobFilter(job)
		assert.False(t, result)
	})
}

// TestPodFilterEdgeCases tests edge cases for PodFilter
func TestPodFilterEdgeCases(t *testing.T) {
	t.Run("pod with multiple PodImageReady conditions", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "DaemonSet",
					},
				},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   daemonsetupgradestrategy.PodImageReady,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   daemonsetupgradestrategy.PodImageReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		result := PodFilter(pod)
		assert.True(t, result) // Should return true if any condition matches
	})

	t.Run("pod with condition type that is not PodImageReady", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   "CustomCondition",
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		result := PodFilter(pod)
		assert.False(t, result)
	})
}
