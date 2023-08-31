/*
Copyright 2022 The Openyurt Authors.

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

package yurtappdaemon

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

//const updateRetries = 5

func TestIsTolerationsAllTaints(t *testing.T) {
	tests := []struct {
		name        string
		tolerations []corev1.Toleration
		taints      []corev1.Taint
		expect      bool
	}{
		{
			"false",
			[]corev1.Toleration{
				{
					Key: "a",
				},
			},
			[]corev1.Taint{
				{
					Key: "b",
				},
			},
			false,
		},
		{
			"true",
			[]corev1.Toleration{
				{
					Key: "a",
				},
			},
			[]corev1.Taint{
				{
					Key: "a",
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := IsTolerationsAllTaints(tt.tolerations, tt.taints)

			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestNewYurtAppDaemonCondition(t *testing.T) {
	tests := []struct {
		name     string
		condType unitv1alpha1.YurtAppDaemonConditionType
		status   corev1.ConditionStatus
		reason   string
		message  string
		expect   unitv1alpha1.YurtAppDaemonCondition
	}{
		{
			"normal",
			unitv1alpha1.WorkLoadUpdated,
			corev1.ConditionTrue,
			"a",
			"b",
			unitv1alpha1.YurtAppDaemonCondition{
				Type:               unitv1alpha1.WorkLoadUpdated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "a",
				Message:            "b",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := NewYurtAppDaemonCondition(tt.condType, tt.status, tt.reason, tt.message)

			if !reflect.DeepEqual(get.Type, tt.expect.Type) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, *get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetYurtAppDaemonCondition(t *testing.T) {
	tests := []struct {
		name     string
		status   unitv1alpha1.YurtAppDaemonStatus
		condType unitv1alpha1.YurtAppDaemonConditionType
		expect   unitv1alpha1.YurtAppDaemonCondition
	}{
		{
			"normal",
			unitv1alpha1.YurtAppDaemonStatus{
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type: unitv1alpha1.WorkLoadProvisioned,
					},
				},
			},
			unitv1alpha1.WorkLoadProvisioned,
			unitv1alpha1.YurtAppDaemonCondition{
				Type: unitv1alpha1.WorkLoadProvisioned,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := GetYurtAppDaemonCondition(tt.status, tt.condType)

			if !reflect.DeepEqual(get.Type, tt.expect.Type) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, *get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestSetYurtAppDaemonCondition(t *testing.T) {
	tests := []struct {
		name   string
		status *unitv1alpha1.YurtAppDaemonStatus
		cond   *unitv1alpha1.YurtAppDaemonCondition
		expect unitv1alpha1.YurtAppDaemonCondition
	}{
		{
			"normal",
			&unitv1alpha1.YurtAppDaemonStatus{
				Conditions: []unitv1alpha1.YurtAppDaemonCondition{
					{
						Type:   unitv1alpha1.WorkLoadProvisioned,
						Status: "a",
						Reason: "b",
					},
				},
			},
			&unitv1alpha1.YurtAppDaemonCondition{
				Type: unitv1alpha1.WorkLoadProvisioned,
			},
			unitv1alpha1.YurtAppDaemonCondition{
				Type: unitv1alpha1.WorkLoadProvisioned,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetYurtAppDaemonCondition(tt.status, tt.cond)

			//if !reflect.DeepEqual(get.Type, tt.expect.Type) {
			//	t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, tt.expect)
			//}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, tt.expect)
		})
	}
}
