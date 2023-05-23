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

	devicev1alpha2 "github.com/openyurtio/openyurt/pkg/apis/device/v1alpha2"
)

// NewIoTCondition creates a new IoT condition.
func NewIoTCondition(condType devicev1alpha2.IoTConditionType, status corev1.ConditionStatus, reason, message string) *devicev1alpha2.IoTCondition {
	return &devicev1alpha2.IoTCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetIoTCondition returns the condition with the provided type.
func GetIoTCondition(status devicev1alpha2.IoTStatus, condType devicev1alpha2.IoTConditionType) *devicev1alpha2.IoTCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetIoTCondition updates the IoT to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetIoTCondition(status *devicev1alpha2.IoTStatus, condition *devicev1alpha2.IoTCondition) {
	currentCond := GetIoTCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

func filterOutCondition(conditions []devicev1alpha2.IoTCondition, condType devicev1alpha2.IoTConditionType) []devicev1alpha2.IoTCondition {
	var newConditions []devicev1alpha2.IoTCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
