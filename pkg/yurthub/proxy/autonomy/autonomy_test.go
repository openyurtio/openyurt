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

package autonomy

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func TestSetNodeAutonomyCondition(t *testing.T) {
	testcases := []struct {
		name           string
		node           *v1.Node
		expectedStatus v1.ConditionStatus
	}{
		{
			name: "case1",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
		{
			name: "case2",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   appsv1beta1.NodeAutonomy,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
		{
			name: "case3",
			node: &v1.Node{
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   appsv1beta1.NodeAutonomy,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expectedStatus: v1.ConditionTrue,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			setNodeAutonomyCondition(tc.node, tc.expectedStatus, "", "")
			for _, condition := range tc.node.Status.Conditions {
				if condition.Type == appsv1beta1.NodeAutonomy && condition.Status != tc.expectedStatus {
					t.Error("failed to set node autonomy status")
				}
			}
		})
	}
}
