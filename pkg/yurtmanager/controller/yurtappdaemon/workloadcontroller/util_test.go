/*
Copyright 2021 The OpenYurt Authors.

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

package workloadcontroller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestGetWorkloadPrefix(t *testing.T) {
	tests := []struct {
		name           string
		controllerName string
		nodepoolName   string
		expect         string
	}{
		{
			"true",
			"a",
			"b",
			"a-b-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := getWorkloadPrefix(tt.controllerName, tt.nodepoolName)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestCreateNodeSelectorByNodepoolName(t *testing.T) {
	tests := []struct {
		name     string
		nodepool string
		expect   map[string]string
	}{
		{
			"normal",
			"a",
			map[string]string{
				projectinfo.GetNodePoolLabel(): "a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := CreateNodeSelectorByNodepoolName(tt.nodepool)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestTaintsToTolerations(t *testing.T) {
	tests := []struct {
		name   string
		taints []corev1.Taint
		expect []corev1.Toleration
	}{
		{
			"normal",
			[]corev1.Taint{
				{
					Key:    "a",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			[]corev1.Toleration{
				{
					Key:      "a",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := TaintsToTolerations(tt.taints)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}
