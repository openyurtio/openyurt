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
	"testing"

	"github.com/stretchr/testify/assert"

	unitv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func TestFilterOutCondition(t *testing.T) {
	// 测试用例1: 空条件列表
	conditions := []unitv1beta1.YurtAppSetCondition{}
	expectedNewConditions := []unitv1beta1.YurtAppSetCondition{}
	outCondition, actualConditions := filterOutCondition(conditions, unitv1beta1.AppSetAppDispatchced)
	assert.Equal(t, expectedNewConditions, actualConditions)
	assert.Nil(t, outCondition)

	// 测试用例2: 条件列表中没有指定类型
	conditions = []unitv1beta1.YurtAppSetCondition{
		{Type: unitv1beta1.AppSetPoolFound},
		{Type: unitv1beta1.AppSetAppDispatchced},
	}
	expectedNewConditions = []unitv1beta1.YurtAppSetCondition{
		{Type: unitv1beta1.AppSetPoolFound},
		{Type: unitv1beta1.AppSetAppDispatchced},
	}
	outCondition, actualConditions = filterOutCondition(conditions, unitv1beta1.AppSetAppDeleted)
	assert.Equal(t, expectedNewConditions, actualConditions)
	assert.Nil(t, outCondition)

	// 测试用例3: 条件列表中有指定类型
	matchedCondition := &unitv1beta1.YurtAppSetCondition{Type: unitv1beta1.AppSetAppDeleted}
	conditions = []unitv1beta1.YurtAppSetCondition{
		{Type: unitv1beta1.AppSetPoolFound},
		*matchedCondition,
		{Type: unitv1beta1.AppSetAppDispatchced},
	}
	expectedNewConditions = []unitv1beta1.YurtAppSetCondition{
		{Type: unitv1beta1.AppSetPoolFound},
		{Type: unitv1beta1.AppSetAppDispatchced},
	}
	outCondition, actualConditions = filterOutCondition(conditions, unitv1beta1.AppSetAppDeleted)
	assert.Equal(t, expectedNewConditions, actualConditions)
	assert.Equal(t, outCondition, matchedCondition)
}

func TestSetYurtAppSetCondition(t *testing.T) {
	// 测试用例1: 当条件已经存在且状态和原因相同，则更新
	var status unitv1beta1.YurtAppSetStatus = unitv1beta1.YurtAppSetStatus{
		Conditions: []unitv1beta1.YurtAppSetCondition{
			{Type: unitv1beta1.AppSetPoolFound, Status: "True", Reason: "TestReason1", Message: "TestMessage2"},
		},
	}
	condition1 := NewYurtAppSetCondition(unitv1beta1.AppSetPoolFound, "True", "TestReason1", "TestMessage1")
	SetYurtAppSetCondition(&status, condition1)
	assert.Equal(t, status.Conditions[0].Message, "TestMessage1")

	// 测试用例2: 当条件不存在则更新
	condition2 := NewYurtAppSetCondition(unitv1beta1.AppSetAppDeleted, "False", "TestReason2", "TestMessage2")
	SetYurtAppSetCondition(&status, condition2)
	assert.Equal(t, len(status.Conditions), 2)

	// 测试用例3: 当条件已经存在且状态和原因不同，则更新
	condition3 := NewYurtAppSetCondition(unitv1beta1.AppSetAppDeleted, "True", "TestReason3", "TestMessage3")
	SetYurtAppSetCondition(&status, condition3)
	assert.Equal(t, string(status.Conditions[1].Status), "True")
}
