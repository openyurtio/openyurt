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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1helper "k8s.io/component-helpers/scheduling/corev1"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const updateRetries = 5

func IsTolerationsAllTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
	for i := range taints {
		if !v1helper.TolerationsTolerateTaint(tolerations, &taints[i]) {
			return false
		}
	}
	return true
}

// NewYurtAppDaemonCondition creates a new YurtAppDaemon condition.
func NewYurtAppDaemonCondition(condType unitv1alpha1.YurtAppDaemonConditionType, status corev1.ConditionStatus, reason, message string) *unitv1alpha1.YurtAppDaemonCondition {
	return &unitv1alpha1.YurtAppDaemonCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetYurtAppDaemonCondition returns the condition with the provided type.
func GetYurtAppDaemonCondition(status unitv1alpha1.YurtAppDaemonStatus, condType unitv1alpha1.YurtAppDaemonConditionType) *unitv1alpha1.YurtAppDaemonCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// RemoveYurtAppDaemonCondition removes the YurtAppDaemon condition with the provided type.
func RemoveYurtAppDaemonCondition(status *unitv1alpha1.YurtAppDaemonStatus, condType unitv1alpha1.YurtAppDaemonConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

// SetYurtAppDaemonCondition updates the YurtAppDaemon to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetYurtAppDaemonCondition(status *unitv1alpha1.YurtAppDaemonStatus, condition *unitv1alpha1.YurtAppDaemonCondition) {
	currentCond := GetYurtAppDaemonCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

func filterOutCondition(conditions []unitv1alpha1.YurtAppDaemonCondition, condType unitv1alpha1.YurtAppDaemonConditionType) []unitv1alpha1.YurtAppDaemonCondition {
	var newConditions []unitv1alpha1.YurtAppDaemonCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
