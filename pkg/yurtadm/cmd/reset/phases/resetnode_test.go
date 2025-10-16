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

package phases

import (
	"os/exec"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

func Test_runStopYurthubService_Success(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("true")
	}

	if err := runStopYurthubService(); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_runStopYurthubService_StopFails(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("false")
	}

	if err := runStopYurthubService(); err == nil {
		t.Fatalf("expected error when stop fails, got nil")
	}
}

func Test_runStopYurthubService_DisableFails(t *testing.T) {
	old := execCommand
	defer func() { execCommand = old }()

	call := 0
	execCommand = func(name string, arg ...string) *exec.Cmd {
		call++
		if call == 1 {
			if name != "systemctl" || len(arg) < 2 || arg[0] != "stop" {
				return exec.Command("true")
			}
			return exec.Command("true")
		}
		if name == "systemctl" && len(arg) > 0 && arg[0] == "disable" {
			if len(arg) >= 2 && arg[1] == constants.YurtHubServiceName {
				return exec.Command("false")
			}
		}
		return exec.Command("true")
	}

	if err := runStopYurthubService(); err == nil {
		t.Fatalf("expected error when disable fails, got nil")
	}
}
