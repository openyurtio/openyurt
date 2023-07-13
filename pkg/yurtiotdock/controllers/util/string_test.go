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

package util

import (
	"testing"
)

func TestIsInStringLst(t *testing.T) {
	tests := []struct {
		desc string
		sl   []string
		s    string
		res  bool
	}{
		{
			"test empty list",
			[]string{},
			"a",
			false,
		},
		{
			"test not in list",
			[]string{"a", "b", "c"},
			"d",
			false,
		},
		{
			"test not in list with one element",
			[]string{"a"},
			"b",
			false,
		},
		{
			"test in list with one element",
			[]string{"aaa"},
			"aaa",
			true,
		},
		{
			"test in list with one element",
			[]string{"aaa", "a", "bbb"},
			"a",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			res := IsInStringLst(tt.sl, tt.s)
			if res != tt.res {
				t.Errorf("expect %v, but %v returned", tt.res, res)
			}
		})
	}
}
