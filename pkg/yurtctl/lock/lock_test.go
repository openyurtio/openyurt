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

package lock

import (
	"testing"
	"time"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name         string
		oldTime      int64
		expectResult bool
	}{
		{"timeoutCase1", time.Now().Unix() - (LockTimeoutMin+1)*60, true},
		{"timeoutCase2", time.Now().Unix() - (LockTimeoutMin*60 + 10), true},
		{"notTimeoutCase1", time.Now().Unix() - (LockTimeoutMin-1)*60, false},
		{"notTimeoutCase2", time.Now().Unix() - (LockTimeoutMin*60 - 10), false},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				result := isTimeout(st.oldTime)
				if result != st.expectResult {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expectResult, result)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expectResult, result)
			}
		}
		t.Run(st.name, tf)
	}
}
