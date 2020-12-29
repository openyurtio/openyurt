/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

@CHANGELOG
OpenYurt Authors:
change some functions
*/

package uniteddeployment

import (
	"fmt"

	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const updateRetries = 5

func getPoolNameFrom(metaObj metav1.Object) (string, error) {
	name, exist := metaObj.GetLabels()[unitv1alpha1.PoolNameLabelKey]
	if !exist {
		return "", fmt.Errorf("fail to get pool name from label of pool %s/%s: no label %s found", metaObj.GetNamespace(), metaObj.GetName(), unitv1alpha1.PoolNameLabelKey)
	}

	if len(name) == 0 {
		return "", fmt.Errorf("fail to get pool name from label of pool %s/%s: label %s has an empty value", metaObj.GetNamespace(), metaObj.GetName(), unitv1alpha1.PoolNameLabelKey)
	}

	return name, nil
}

// NewUnitedDeploymentCondition creates a new UnitedDeployment condition.
func NewUnitedDeploymentCondition(condType unitv1alpha1.UnitedDeploymentConditionType, status corev1.ConditionStatus, reason, message string) *unitv1alpha1.UnitedDeploymentCondition {
	return &unitv1alpha1.UnitedDeploymentCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetUnitedDeploymentCondition returns the condition with the provided type.
func GetUnitedDeploymentCondition(status unitv1alpha1.UnitedDeploymentStatus, condType unitv1alpha1.UnitedDeploymentConditionType) *unitv1alpha1.UnitedDeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetUnitedDeploymentCondition updates the UnitedDeployment to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetUnitedDeploymentCondition(status *unitv1alpha1.UnitedDeploymentStatus, condition *unitv1alpha1.UnitedDeploymentCondition) {
	currentCond := GetUnitedDeploymentCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

// RemoveUnitedDeploymentCondition removes the UnitedDeployment condition with the provided type.
func RemoveUnitedDeploymentCondition(status *unitv1alpha1.UnitedDeploymentStatus, condType unitv1alpha1.UnitedDeploymentConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

func filterOutCondition(conditions []unitv1alpha1.UnitedDeploymentCondition, condType unitv1alpha1.UnitedDeploymentConditionType) []unitv1alpha1.UnitedDeploymentCondition {
	var newConditions []unitv1alpha1.UnitedDeploymentCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
