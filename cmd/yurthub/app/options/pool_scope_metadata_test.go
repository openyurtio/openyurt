/*
Copyright 2025 The OpenYurt Authors.

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

package options

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSet(t *testing.T) {
	testcases := map[string]struct {
		input     string
		expected  []schema.GroupVersionResource
		expectErr bool
	}{
		"single pool scope metadata": {
			input: "group1/v1/resource1",
			expected: []schema.GroupVersionResource{
				{Group: "group1", Version: "v1", Resource: "resource1"},
			},
		},
		"multiple pool scope metadatas": {
			input: "group1/v1/resource1, group2/v2/resource2",
			expected: []schema.GroupVersionResource{
				{Group: "group1", Version: "v1", Resource: "resource1"},
				{Group: "group2", Version: "v2", Resource: "resource2"},
			},
		},
		"multiple pool scope metadatas with empty group": {
			input: "/v1/resource1, /v2/resource2",
			expected: []schema.GroupVersionResource{
				{Group: "", Version: "v1", Resource: "resource1"},
				{Group: "", Version: "v2", Resource: "resource2"},
			},
		},
		"invalid format of pool scope metadata": {
			input:     "group1/v1",
			expectErr: true,
		},
		"empty string of pool scope metadata": {
			input:     "",
			expectErr: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			var psm PoolScopeMetadatas
			err := psm.Set(tc.input)
			if (err != nil) != tc.expectErr {
				t.Errorf("expected error %v, but got %v", tc.expectErr, err != nil)
			}

			if !tc.expectErr && !comparePoolScopeMetadatas(psm, tc.expected) {
				t.Errorf("expected pool scope metadatas: %+v, but got %+v", tc.expected, psm)
			}
		})
	}
}

func comparePoolScopeMetadatas(a, b []schema.GroupVersionResource) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestString(t *testing.T) {
	testcases := map[string]struct {
		psm         PoolScopeMetadatas
		expectedStr string
	}{
		"single pool scope metadata": {
			psm: PoolScopeMetadatas{
				{Group: "group1", Version: "v1", Resource: "resource1"},
			},
			expectedStr: "group1/v1/resource1",
		},
		"multiple pool scope metadatas": {
			psm: PoolScopeMetadatas{
				{Group: "group1", Version: "v1", Resource: "resource1"},
				{Group: "group2", Version: "v2", Resource: "resource2"},
			},

			expectedStr: "group1/v1/resource1,group2/v2/resource2",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			if tc.psm.String() != tc.expectedStr {
				t.Errorf("expected string %s, but got %s", tc.expectedStr, tc.psm.String())
			}
		})
	}
}
