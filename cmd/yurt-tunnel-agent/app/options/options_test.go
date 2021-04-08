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

package options

import "testing"

func TestAgentIdentifiersAreValid(t *testing.T) {
	testcases := map[string]struct {
		agentIdentifiers string
		result           bool
	}{
		"empty agent identifiers": {
			"",
			true,
		},

		"valid agent identifiers": {
			"host=node-test",
			true,
		},

		"invalid agent identifiers without value": {
			"host",
			false,
		},

		"invalid agent identifiers with invalid key": {
			"foo=node-test",
			false,
		},
	}

	for k, tc := range testcases {
		result := agentIdentifiersAreValid(tc.agentIdentifiers)
		if result != tc.result {
			t.Errorf("%s: agent identifiers are valid  verification result, expect %v, but got %v", k, tc.result, result)
		}
	}
}
