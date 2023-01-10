/*
Copyright 2022 The OpenYurt Authors.

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

package util

import (
	"errors"
	"testing"
)

func TestIsNil(t *testing.T) {
	var nilErr error
	realErr := errors.New("test")

	testcases := map[string]struct {
		inputInterface interface{}
		result         bool
	}{
		"input nil": {
			inputInterface: nil,
			result:         true,
		},
		"nil interface": {
			inputInterface: nilErr,
			result:         true,
		},
		"non-nil interface": {
			inputInterface: realErr,
			result:         false,
		},
		"input struct": {
			inputInterface: struct{}{},
			result:         false,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			got := IsNil(tt.inputInterface)
			if got != tt.result {
				t.Errorf("expect bool result is %v, but got %v", tt.result, got)
			}
		})
	}
}
