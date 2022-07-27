/*
Copyright 2022 The OpenYurt Authors.
Copyright 2015 The Kubernetes Authors.

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

/*
This file was derived from k8s.io/kubernetes/pkg/util/node/node.go
at commit: 27522a29feb.

CHANGELOG from OpenYurt Authors:
1. Pick TestDeleteTaint, TestRemoveTaint, TestAddOrUpdateTaint, TestTaintExists.
2. Add TestTaintSetFilter which is picked from f31bf3ff1.
*/

package taints

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestDeleteTaint(t *testing.T) {
	cases := []struct {
		name           string
		taints         []v1.Taint
		taintToDelete  *v1.Taint
		expectedTaints []v1.Taint
		expectedResult bool
	}{
		{
			name: "delete taint with different name",
			taints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			taintToDelete: &v1.Taint{Key: "foo_1", Effect: v1.TaintEffectNoSchedule},
			expectedTaints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedResult: false,
		},
		{
			name: "delete taint with different effect",
			taints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			taintToDelete: &v1.Taint{Key: "foo", Effect: v1.TaintEffectNoExecute},
			expectedTaints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedResult: false,
		},
		{
			name: "delete taint successfully",
			taints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			taintToDelete:  &v1.Taint{Key: "foo", Effect: v1.TaintEffectNoSchedule},
			expectedTaints: []v1.Taint{},
			expectedResult: true,
		},
		{
			name:           "delete taint from empty taint array",
			taints:         []v1.Taint{},
			taintToDelete:  &v1.Taint{Key: "foo", Effect: v1.TaintEffectNoSchedule},
			expectedTaints: []v1.Taint{},
			expectedResult: false,
		},
	}

	for _, c := range cases {
		taints, result := DeleteTaint(c.taints, c.taintToDelete)
		if result != c.expectedResult {
			t.Errorf("[%s] should return %t, but got: %t", c.name, c.expectedResult, result)
		}
		if !reflect.DeepEqual(taints, c.expectedTaints) {
			t.Errorf("[%s] the result taints should be %v, but got: %v", c.name, c.expectedTaints, taints)
		}
	}
}

func TestRemoveTaint(t *testing.T) {
	cases := []struct {
		name           string
		node           *v1.Node
		taintToRemove  *v1.Taint
		expectedTaints []v1.Taint
		expectedResult bool
	}{
		{
			name: "remove taint unsuccessfully",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key:    "foo",
							Effect: v1.TaintEffectNoSchedule,
						},
					},
				},
			},
			taintToRemove: &v1.Taint{
				Key:    "foo_1",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectedTaints: []v1.Taint{
				{
					Key:    "foo",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedResult: false,
		},
		{
			name: "remove taint successfully",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key:    "foo",
							Effect: v1.TaintEffectNoSchedule,
						},
					},
				},
			},
			taintToRemove: &v1.Taint{
				Key:    "foo",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectedTaints: []v1.Taint{},
			expectedResult: true,
		},
		{
			name: "remove taint from node with no taint",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{},
				},
			},
			taintToRemove: &v1.Taint{
				Key:    "foo",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectedTaints: []v1.Taint{},
			expectedResult: false,
		},
	}

	for _, c := range cases {
		newNode, result, err := RemoveTaint(c.node, c.taintToRemove)
		if err != nil {
			t.Errorf("[%s] should not raise error but got: %v", c.name, err)
		}
		if result != c.expectedResult {
			t.Errorf("[%s] should return %t, but got: %t", c.name, c.expectedResult, result)
		}
		if !reflect.DeepEqual(newNode.Spec.Taints, c.expectedTaints) {
			t.Errorf("[%s] the new node object should have taints %v, but got: %v", c.name, c.expectedTaints, newNode.Spec.Taints)
		}
	}
}

func TestAddOrUpdateTaint(t *testing.T) {
	node := &v1.Node{}

	taint := &v1.Taint{
		Key:    "foo",
		Value:  "bar",
		Effect: v1.TaintEffectNoSchedule,
	}

	checkResult := func(testCaseName string, newNode *v1.Node, expectedTaint *v1.Taint, result, expectedResult bool, err error) {
		if err != nil {
			t.Errorf("[%s] should not raise error but got %v", testCaseName, err)
		}
		if result != expectedResult {
			t.Errorf("[%s] should return %t, but got: %t", testCaseName, expectedResult, result)
		}
		if len(newNode.Spec.Taints) != 1 || !reflect.DeepEqual(newNode.Spec.Taints[0], *expectedTaint) {
			t.Errorf("[%s] node should only have one taint: %v, but got: %v", testCaseName, *expectedTaint, newNode.Spec.Taints)
		}
	}

	// Add a new Taint.
	newNode, result, err := AddOrUpdateTaint(node, taint)
	checkResult("Add New Taint", newNode, taint, result, true, err)

	// Update a Taint.
	taint.Value = "bar_1"
	newNode, result, err = AddOrUpdateTaint(node, taint)
	checkResult("Update Taint", newNode, taint, result, true, err)

	// Add a duplicate Taint.
	node = newNode
	newNode, result, err = AddOrUpdateTaint(node, taint)
	checkResult("Add Duplicate Taint", newNode, taint, result, false, err)
}

func TestTaintExists(t *testing.T) {
	testingTaints := []v1.Taint{
		{
			Key:    "foo_1",
			Value:  "bar_1",
			Effect: v1.TaintEffectNoExecute,
		},
		{
			Key:    "foo_2",
			Value:  "bar_2",
			Effect: v1.TaintEffectNoSchedule,
		},
	}

	cases := []struct {
		name           string
		taintToFind    *v1.Taint
		expectedResult bool
	}{
		{
			name:           "taint exists",
			taintToFind:    &v1.Taint{Key: "foo_1", Value: "bar_1", Effect: v1.TaintEffectNoExecute},
			expectedResult: true,
		},
		{
			name:           "different key",
			taintToFind:    &v1.Taint{Key: "no_such_key", Value: "bar_1", Effect: v1.TaintEffectNoExecute},
			expectedResult: false,
		},
		{
			name:           "different effect",
			taintToFind:    &v1.Taint{Key: "foo_1", Value: "bar_1", Effect: v1.TaintEffectNoSchedule},
			expectedResult: false,
		},
	}

	for _, c := range cases {
		result := TaintExists(testingTaints, c.taintToFind)

		if result != c.expectedResult {
			t.Errorf("[%s] unexpected results: %v", c.name, result)
			continue
		}
	}
}

func TestTaintSetDiff(t *testing.T) {
	cases := []struct {
		name                   string
		t1                     []v1.Taint
		t2                     []v1.Taint
		expectedTaintsToAdd    []*v1.Taint
		expectedTaintsToRemove []*v1.Taint
	}{
		{
			name:                   "two_taints_are_nil",
			expectedTaintsToAdd:    nil,
			expectedTaintsToRemove: nil,
		},
		{
			name: "one_taint_is_nil_and_the_other_is_not_nil",
			t1: []v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedTaintsToAdd: []*v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedTaintsToRemove: nil,
		},
		{
			name: "shared_taints_with_the_same_key_value_effect",
			t1: []v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			t2: []v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedTaintsToAdd: []*v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
			},
			expectedTaintsToRemove: []*v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
			},
		},
		{
			name: "shared_taints_with_the_same_key_effect_different_value",
			t1: []v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "different-value",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			t2: []v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedTaintsToAdd: []*v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
			},
			expectedTaintsToRemove: []*v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
			},
		},
		{
			name: "shared_taints_with_the_same_key_different_value_effect",
			t1: []v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "different-value",
					Effect: v1.TaintEffectNoExecute,
				},
			},
			t2: []v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			expectedTaintsToAdd: []*v1.Taint{
				{
					Key:    "foo_1",
					Value:  "bar_1",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "different-value",
					Effect: v1.TaintEffectNoExecute,
				},
			},
			expectedTaintsToRemove: []*v1.Taint{
				{
					Key:    "foo_3",
					Value:  "bar_3",
					Effect: v1.TaintEffectNoExecute,
				},
				{
					Key:    "foo_2",
					Value:  "bar_2",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			add, remove := TaintSetDiff(tt.t1, tt.t2)
			if !reflect.DeepEqual(add, tt.expectedTaintsToAdd) {
				t.Errorf("taintsToAdd: %v should equal %v, but get unexpected results", add, tt.expectedTaintsToAdd)
			}
			if !reflect.DeepEqual(remove, tt.expectedTaintsToRemove) {
				t.Errorf("taintsToRemove: %v should equal %v, but get unexpected results", remove, tt.expectedTaintsToRemove)
			}
		})
	}
}
