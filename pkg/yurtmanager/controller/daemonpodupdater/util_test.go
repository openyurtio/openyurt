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

package daemonpodupdater

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDaemonsetPods(t *testing.T) {
	ds1 := newDaemonSet("daemosnet1", "foo/bar:v1")

	pod1 := newPod("pod1", "", simpleDaemonSetLabel, ds1)
	pod2 := newPod("pod2", "", simpleDaemonSetLabel, nil)

	expectPods := []*corev1.Pod{pod1}
	client := fakeclient.NewClientBuilder().WithObjects(ds1, pod1, pod2).Build()

	gotPods, err := GetDaemonsetPods(client, ds1)

	assert.Equal(t, nil, err)
	assert.Equal(t, expectPods, gotPods)
}

func TestIsDaemonsetPodLatest(t *testing.T) {
	daemosnetV1 := newDaemonSet("daemonset", "foo/bar:v1")
	daemosnetV2 := daemosnetV1.DeepCopy()
	daemosnetV2.Spec.Template.Spec.Containers[0].Image = "foo/bar:v2"

	tests := []struct {
		name       string
		ds         *appsv1.DaemonSet
		pod        *corev1.Pod
		wantLatest bool
	}{
		{
			name:       "latest",
			ds:         daemosnetV1,
			pod:        newPod("pod", "", simpleDaemonSetLabel, daemosnetV1),
			wantLatest: true,
		},
		{
			name:       "not latest",
			ds:         daemosnetV2,
			pod:        newPod("pod", "", simpleDaemonSetLabel, daemosnetV1),
			wantLatest: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLatest := IsDaemonsetPodLatest(tt.ds, tt.pod)
			assert.Equal(t, tt.wantLatest, gotLatest)
		})
	}
}

func Test_checkPrerequisites(t *testing.T) {
	tests := []struct {
		name string
		ds   *appsv1.DaemonSet
		want bool
	}{
		{
			name: "satisfied-ota",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"apps.openyurt.io/update-strategy": "OTA",
					},
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.OnDeleteDaemonSetStrategyType,
					},
				},
			},
			want: true,
		},
		{
			name: "satisfied-auto",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"apps.openyurt.io/update-strategy": "Auto",
					},
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.OnDeleteDaemonSetStrategyType,
					},
				},
			},
			want: true,
		},
		{
			name: "unsatisfied-other",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"apps.openyurt.io/update-strategy": "other",
					},
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.OnDeleteDaemonSetStrategyType,
					},
				},
			},
			want: false,
		},
		{
			name: "unsatisfied-without-ann",
			ds: &appsv1.DaemonSet{
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						Type: appsv1.OnDeleteDaemonSetStrategyType,
					},
				},
			},
			want: false,
		},
		{
			name: "unsatisfied-without-updateStrategy",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"apps.openyurt.io/update-strategy": "other",
					},
				},
			},
			want: false,
		},
		{
			name: "unsatisfied-without-both",
			ds:   &appsv1.DaemonSet{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkPrerequisites(tt.ds); got != tt.want {
				t.Errorf("checkPrerequisites() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTargetNodeName(t *testing.T) {
	pod := newPod("pod", "", nil, nil)

	podWithName := pod.DeepCopy()
	podWithName.Spec.NodeName = "node"

	podwithAffinity := pod.DeepCopy()
	podwithAffinity.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchFields: []corev1.NodeSelectorRequirement{
							{
								Key:      metav1.ObjectNameField,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"affinity"},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		pod     *corev1.Pod
		want    string
		wantErr bool
	}{
		{
			pod:     pod,
			want:    "",
			wantErr: true,
		},
		{
			pod:     podWithName,
			want:    "node",
			wantErr: false,
		},
		{
			pod:     podwithAffinity,
			want:    "affinity",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := GetTargetNodeName(tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTargetNodeName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetTargetNodeName() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPodUpgradeCondition(t *testing.T) {
	pod1 := newPod("pod1", "", nil, nil)
	pod1.Status = corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   PodNeedUpgrade,
				Status: corev1.ConditionTrue,
			},
		},
	}

	pod2 := pod1.DeepCopy()
	pod2.Status = corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   PodNeedUpgrade,
				Status: corev1.ConditionFalse,
			},
		},
	}

	tests := []struct {
		name   string
		status corev1.PodStatus
		want   *corev1.PodCondition
	}{
		{
			name:   "pod1",
			status: pod1.Status,
			want: &corev1.PodCondition{
				Type:   PodNeedUpgrade,
				Status: corev1.ConditionTrue,
			},
		},
		{
			name:   "pod2",
			status: pod2.Status,
			want: &corev1.PodCondition{
				Type:   PodNeedUpgrade,
				Status: corev1.ConditionFalse,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPodUpgradeCondition(tt.status); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetPodUpgradeCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}
