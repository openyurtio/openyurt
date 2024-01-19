/*
Copyright 2024 The OpenYurt Authors.

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

package yurtappset

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	unitv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

// NewYurtAppSetCondition creates a new YurtAppSet condition.
func NewYurtAppSetCondition(condType unitv1beta1.YurtAppSetConditionType, status corev1.ConditionStatus, reason, message string) *unitv1beta1.YurtAppSetCondition {
	return &unitv1beta1.YurtAppSetCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// SetYurtAppSetCondition updates the YurtAppSet to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason then we are not going to update.
func SetYurtAppSetCondition(status *unitv1beta1.YurtAppSetStatus, condition *unitv1beta1.YurtAppSetCondition) {
	originalCondition, newConditions := filterOutCondition(status.Conditions, condition.Type)
	if originalCondition != nil &&
		originalCondition.Type == condition.Type &&
		originalCondition.Status == condition.Status &&
		originalCondition.Reason == condition.Reason &&
		originalCondition.Message == condition.Message {
		klog.V(5).Infof("Not updating condition %s status to %s because it is already set to %s", condition.Type, condition.Status, originalCondition.Status)
	} else {
		klog.V(4).Infof("Updating condition %s status to %s: %s", condition.Type, condition.Status, condition.Message)
		status.Conditions = append(newConditions, *condition)
	}
}

// RemoveYurtAppSetCondition removes the YurtAppSet condition with the provided type.
func RemoveYurtAppSetCondition(status *unitv1beta1.YurtAppSetStatus, condType unitv1beta1.YurtAppSetConditionType) {
	_, status.Conditions = filterOutCondition(status.Conditions, condType)
}

// filterOutCondition returns a tuple containing the first matching condition and a new slice of conditions without conditions with the provided type
func filterOutCondition(conditions []unitv1beta1.YurtAppSetCondition, condType unitv1beta1.YurtAppSetConditionType) (outCondition *unitv1beta1.YurtAppSetCondition, newConditions []unitv1beta1.YurtAppSetCondition) {
	newConditions = []unitv1beta1.YurtAppSetCondition{}
	for i, c := range conditions {
		if c.Type == condType {
			outCondition = &conditions[i]
		} else {
			newConditions = append(newConditions, c)
		}
	}
	return outCondition, newConditions
}
