/*
Copyright 2021 The OpenYurt Authors.
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

package yurtappset

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const updateRetries = 5

type YurtAppSetPatches struct {
	Replicas int32
	Patch    string
}

func getPoolNameFrom(metaObj metav1.Object) (string, error) {
	name, exist := metaObj.GetLabels()[apps.PoolNameLabelKey]
	if !exist {
		return "", fmt.Errorf("could not get pool name from label of pool %s/%s: no label %s found", metaObj.GetNamespace(), metaObj.GetName(), apps.PoolNameLabelKey)
	}

	if len(name) == 0 {
		return "", fmt.Errorf("could not get pool name from label of pool %s/%s: label %s has an empty value", metaObj.GetNamespace(), metaObj.GetName(), apps.PoolNameLabelKey)
	}

	return name, nil
}

// NewYurtAppSetCondition creates a new YurtAppSet condition.
func NewYurtAppSetCondition(condType unitv1alpha1.YurtAppSetConditionType, status corev1.ConditionStatus, reason, message string) *unitv1alpha1.YurtAppSetCondition {
	return &unitv1alpha1.YurtAppSetCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetYurtAppSetCondition returns the condition with the provided type.
func GetYurtAppSetCondition(status unitv1alpha1.YurtAppSetStatus, condType unitv1alpha1.YurtAppSetConditionType) *unitv1alpha1.YurtAppSetCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetYurtAppSetCondition updates the YurtAppSet to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetYurtAppSetCondition(status *unitv1alpha1.YurtAppSetStatus, condition *unitv1alpha1.YurtAppSetCondition) {
	currentCond := GetYurtAppSetCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

// RemoveYurtAppSetCondition removes the YurtAppSet condition with the provided type.
func RemoveYurtAppSetCondition(status *unitv1alpha1.YurtAppSetStatus, condType unitv1alpha1.YurtAppSetConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

func filterOutCondition(conditions []unitv1alpha1.YurtAppSetCondition, condType unitv1alpha1.YurtAppSetConditionType) []unitv1alpha1.YurtAppSetCondition {
	var newConditions []unitv1alpha1.YurtAppSetCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

func GetNextPatches(yas *unitv1alpha1.YurtAppSet) map[string]YurtAppSetPatches {
	next := make(map[string]YurtAppSetPatches)
	for _, pool := range yas.Spec.Topology.Pools {
		t := YurtAppSetPatches{}
		if pool.Replicas != nil {
			t.Replicas = *pool.Replicas
		}
		if pool.Patch != nil {
			t.Patch = string(pool.Patch.Raw)
		}
		next[pool.Name] = t
	}
	return next
}
