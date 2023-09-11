/*
Copyright 2015 The Kubernetes Authors.

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

package pod

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

func newPod(now metav1.Time, ready bool, beforeSec int) *v1.Pod {
	conditionStatus := v1.ConditionFalse
	if ready {
		conditionStatus = v1.ConditionTrue
	}
	return &v1.Pod{
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodReady,
					LastTransitionTime: metav1.NewTime(now.Time.Add(-1 * time.Duration(beforeSec) * time.Second)),
					Status:             conditionStatus,
				},
			},
		},
	}
}

func TestIsPodAvailable(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		pod             *v1.Pod
		minReadySeconds int32
		expected        bool
	}{
		{
			pod:             newPod(now, false, 0),
			minReadySeconds: 0,
			expected:        false,
		},
		{
			pod:             newPod(now, true, 0),
			minReadySeconds: 1,
			expected:        false,
		},
		{
			pod:             newPod(now, true, 0),
			minReadySeconds: 0,
			expected:        true,
		},
		{
			pod:             newPod(now, true, 51),
			minReadySeconds: 50,
			expected:        true,
		},
	}

	for i, test := range tests {
		isAvailable := IsPodAvailable(test.pod, test.minReadySeconds, now)
		if isAvailable != test.expected {
			t.Errorf("[tc #%d] expected available pod: %t, got: %t", i, test.expected, isAvailable)
		}
	}
}

func newStaticPod(podName string, nodeName string, namespace string, isStaticPod bool) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ConfigSourceAnnotationKey: "true",
			},
			Name:      podName + "-" + rand.String(10),
			Namespace: namespace,
		},
		Spec: v1.PodSpec{NodeName: nodeName},
	}

	if isStaticPod {
		pod.Name = podName + "-" + nodeName
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{Kind: "Node"}}
	} else {
		pod.Annotations = nil
	}

	return pod
}

func TestIsStaticPod(t *testing.T) {
	tests := []struct {
		name string
		pod  *v1.Pod
		want bool
	}{
		{
			name: "test1",
			pod:  newStaticPod("test1", "test1", "", true),
			want: true,
		},
		{
			name: "test2",
			pod:  newStaticPod("test2", "test2", "", false),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStaticPod(tt.pod); got != tt.want {
				t.Errorf("IsStaticPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdatePodCondition(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name      string
		status    *v1.PodStatus
		condition *v1.PodCondition
		want      bool
	}{
		{
			name:      "test1",
			status:    &newPod(now, false, 0).Status,
			condition: &v1.PodCondition{},
			want:      true,
		},
		{
			name:   "test2",
			status: &newPod(now, true, 0).Status,
			condition: &v1.PodCondition{
				Type: v1.PodReady,
			},
			want: true,
		},
		{
			name:   "test3",
			status: &newPod(now, true, 0).Status,
			condition: &v1.PodCondition{
				Type:   v1.PodReady,
				Status: "True",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdatePodCondition(tt.status, tt.condition); got != tt.want {
				t.Errorf("UpdatePodCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}
