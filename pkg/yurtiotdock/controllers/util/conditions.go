/*
Copyright 2023 The OpenYurt Authors.

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

package util

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

// NewDeviceCondition creates a new Device condition.
func NewDeviceCondition(condType iotv1alpha1.DeviceConditionType, status corev1.ConditionStatus, reason, message string) *iotv1alpha1.DeviceCondition {
	return &iotv1alpha1.DeviceCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetDeviceCondition returns the condition with the provided type.
func GetDeviceCondition(status iotv1alpha1.DeviceStatus, condType iotv1alpha1.DeviceConditionType) *iotv1alpha1.DeviceCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetDeviceCondition updates the Device to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetDeviceCondition(status *iotv1alpha1.DeviceStatus, condition *iotv1alpha1.DeviceCondition) {
	currentCond := GetDeviceCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutDeviceCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

func filterOutDeviceCondition(conditions []iotv1alpha1.DeviceCondition, condType iotv1alpha1.DeviceConditionType) []iotv1alpha1.DeviceCondition {
	var newConditions []iotv1alpha1.DeviceCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

// NewDeviceServiceCondition creates a new DeviceService condition.
func NewDeviceServiceCondition(condType iotv1alpha1.DeviceServiceConditionType, status corev1.ConditionStatus, reason, message string) *iotv1alpha1.DeviceServiceCondition {
	return &iotv1alpha1.DeviceServiceCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetDeviceServiceCondition returns the condition with the provided type.
func GetDeviceServiceCondition(status iotv1alpha1.DeviceServiceStatus, condType iotv1alpha1.DeviceServiceConditionType) *iotv1alpha1.DeviceServiceCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetDeviceServiceCondition updates the DeviceService to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetDeviceServiceCondition(status *iotv1alpha1.DeviceServiceStatus, condition *iotv1alpha1.DeviceServiceCondition) {
	currentCond := GetDeviceServiceCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutDeviceServiceCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

func filterOutDeviceServiceCondition(conditions []iotv1alpha1.DeviceServiceCondition, condType iotv1alpha1.DeviceServiceConditionType) []iotv1alpha1.DeviceServiceCondition {
	var newConditions []iotv1alpha1.DeviceServiceCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
