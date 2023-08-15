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

package utils

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/constant"
)

func TestDeleteTaintsByKey(t *testing.T) {
	taints := []v1.Taint{
		{
			Key:    constant.NodeNotSchedulableTaint,
			Value:  "true",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
	_, d := DeleteTaintsByKey(taints, "key")
	if d == true {
		t.Fail()
	}
	_, d = DeleteTaintsByKey(taints, constant.NodeNotSchedulableTaint)
	if d != true {
		t.Fail()
	}
}

func TestTaintKeyExists(t *testing.T) {
	taints := []v1.Taint{
		{
			Key:    constant.NodeNotSchedulableTaint,
			Value:  "true",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
	if TaintKeyExists(taints, "key") != false {
		t.Fail()
	}
	if TaintKeyExists(taints, constant.NodeNotSchedulableTaint) != true {
		t.Fail()
	}
}
