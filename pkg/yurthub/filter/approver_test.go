/*
Copyright 2020 The OpenYurt Authors.

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

package filter

import (
	"testing"
)

func TestApprove(t *testing.T) {
	testcases := map[string]struct {
		comp           string
		resource       string
		verbs          []string
		comp2          string
		resource2      string
		verb2          string
		expectedResult bool
	}{
		"normal case": {
			"kubelet", "services", []string{"list", "watch"},
			"kubelet", "services", "list",
			true,
		},
		"components are not equal": {
			"kubelet", "services", []string{"list", "watch"},
			"kube-proxy", "services", "list",
			false,
		},
		"resources are not equal": {
			"kubelet", "services", []string{"list", "watch"},
			"kubelet", "pods", "list",
			false,
		},
		"verb is not in verbs set": {
			"kubelet", "services", []string{"list", "watch"},
			"kubelet", "services", "get",
			false,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			approver := NewApprover(tt.comp, tt.resource, tt.verbs...)
			result := approver.Approve(tt.comp2, tt.resource2, tt.verb2)

			if result != tt.expectedResult {
				t.Errorf("Approve error: expected %v, but got %v\n", tt.expectedResult, result)
			}
		})
	}
}
