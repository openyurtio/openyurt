/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import "testing"

func TestIsControllerEnabled(t *testing.T) {
	testcases := map[string]struct {
		name        string
		controllers []string
		result      bool
	}{
		"enable specified controller": {
			name:        "foo",
			controllers: []string{"foo", "bar"},
			result:      true,
		},
		"disable specified controller": {
			name:        "foo",
			controllers: []string{"-foo", "bar"},
			result:      false,
		},
		"enable controller in default": {
			name:        "foo",
			controllers: []string{"bar", "*"},
			result:      true,
		},
		"controller doesn't exist": {
			name:        "unknown",
			controllers: []string{"foo", "bar"},
			result:      false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			result := IsControllerEnabled(tc.name, tc.controllers)
			if tc.result != result {
				t.Errorf("expect controller enabled: %v, but got %v", tc.result, result)
			}
		})
	}
}
