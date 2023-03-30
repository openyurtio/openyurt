/*
Copyright 2023 The OpenYurt Authors.
Copyright 2016 The Kubernetes Authors.

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

	"github.com/pkg/errors"
)

type pferror struct{}

func (p *pferror) Preflight() bool { return true }
func (p *pferror) Error() string   { return "" }
func TestCheckErr(t *testing.T) {
	var codeReturned int
	errHandle := func(err string, code int) {
		codeReturned = code
	}

	var tests = []struct {
		name     string
		e        error
		expected int
	}{
		{"error is nil", nil, 0},
		{"empty error", errors.New(""), DefaultErrorExitCode},
		{"preflight error", &pferror{}, PreFlightExitCode},
	}

	for _, rt := range tests {
		t.Run(rt.name, func(t *testing.T) {
			codeReturned = 0
			checkErr(rt.e, errHandle)
			if codeReturned != rt.expected {
				t.Errorf(
					"failed checkErr:\n\texpected: %d\n\t  actual: %d",
					rt.expected,
					codeReturned,
				)
			}
		})
	}
}
