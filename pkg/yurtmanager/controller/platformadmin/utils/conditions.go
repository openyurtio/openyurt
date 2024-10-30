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

package util

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iotv1beta1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
)

// NewPlatformAdminCondition creates a new PlatformAdmin condition.
func NewPlatformAdminCondition(condType iotv1beta1.PlatformAdminConditionType, status corev1.ConditionStatus, reason, message string) *iotv1beta1.PlatformAdminCondition {
	return &iotv1beta1.PlatformAdminCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetPlatformAdminCondition returns the condition with the provided type.
func GetPlatformAdminCondition(status iotv1beta1.PlatformAdminStatus, condType iotv1beta1.PlatformAdminConditionType) *iotv1beta1.PlatformAdminCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetPlatformAdminCondition updates the PlatformAdmin to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetPlatformAdminCondition(status *iotv1beta1.PlatformAdminStatus, condition *iotv1beta1.PlatformAdminCondition) {
	currentCond := GetPlatformAdminCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

func filterOutCondition(conditions []iotv1beta1.PlatformAdminCondition, condType iotv1beta1.PlatformAdminConditionType) []iotv1beta1.PlatformAdminCondition {
	var newConditions []iotv1beta1.PlatformAdminCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
