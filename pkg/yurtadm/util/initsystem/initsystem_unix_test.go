//go:build !windows
// +build !windows

/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package initsystem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupFakeCommand is a helper function that dynamically creates fake 'systemctl' and 'rc-service'
// executables for testing purposes.
func setupFakeCommand(t *testing.T) {
	t.Helper()

	// 1. The source code for our smarter fake command. It can decide its behavior based on arguments.
	const fakeCommandSource = `
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Read an environment variable that tells us which specific command should fail.
	failureTarget := os.Getenv("SIMULATE_FAILURE_FOR")

	// Get the command's own name (e.g., "systemctl") and its arguments.
	commandName := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]
	argString := strings.Join(args, " ")

	// Construct the full command string, e.g., "systemctl daemon-reload".
	fullCmd := commandName + " " + argString

	// Check if the currently invoked command is the one we're supposed to fail.
	if failureTarget == fullCmd {
		fmt.Fprintf(os.Stderr, "simulating failure for: %s", fullCmd)
		os.Exit(1) // Return a non-zero exit code to indicate failure.
	}

	// If it's not the target for failure, simulate success.
	fmt.Fprintf(os.Stdout, "successfully executed: %s", fullCmd)
	os.Exit(0)
}
`
	tempDir := t.TempDir()
	sourceFilePath := filepath.Join(tempDir, "fake_cmd.go")
	if err := os.WriteFile(sourceFilePath, []byte(fakeCommandSource), 0644); err != nil {
		t.Fatalf("Failed to write fake command source: %v", err)
	}

	// Compile the source code into two different executables.
	for _, cmdName := range []string{"systemctl", "rc-service"} {
		fakeCmdPath := filepath.Join(tempDir, cmdName)
		cmd := exec.Command("go", "build", "-o", fakeCmdPath, sourceFilePath)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to build fake command '%s': %v", cmdName, err)
		}
	}

	// Prepend the temporary directory to the PATH.
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", tempDir, os.PathListSeparator, originalPath))
}

func TestSystemdInitSystem(t *testing.T) {
	setupFakeCommand(t)
	sysd := SystemdInitSystem{}
	serviceName := "kubelet"

	t.Run("ServiceStart", func(t *testing.T) {
		t.Run("should succeed when both reload and start succeed", func(t *testing.T) {
			// Set SIMULATE_FAILURE_FOR to a value that will never match, ensuring all commands succeed.
			t.Setenv("SIMULATE_FAILURE_FOR", "none")
			err := sysd.ServiceStart(serviceName)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})

		t.Run("should fail if reloadSystemd fails", func(t *testing.T) {
			// Precisely tell the fake command to fail only when it is 'daemon-reload'.
			t.Setenv("SIMULATE_FAILURE_FOR", "systemctl daemon-reload")
			err := sysd.ServiceStart(serviceName)
			if err == nil {
				t.Error("expected an error for reload failure, but got nil")
			}
		})

		t.Run("should fail if the final start command fails", func(t *testing.T) {
			// Precisely tell the fake command to fail only when it is 'start kubelet'.
			// The 'daemon-reload' call will succeed because it doesn't match.
			t.Setenv("SIMULATE_FAILURE_FOR", fmt.Sprintf("systemctl start %s", serviceName))
			err := sysd.ServiceStart(serviceName)
			if err == nil {
				t.Error("expected an error for start failure, but got nil")
			}
		})
	})

	t.Run("ServiceStop", func(t *testing.T) {
		t.Run("should succeed", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", "none")
			err := sysd.ServiceStop(serviceName)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})

		t.Run("should fail when stop command fails", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", fmt.Sprintf("systemctl stop %s", serviceName))
			err := sysd.ServiceStop(serviceName)
			if err == nil {
				t.Error("expected an error, but got nil")
			}
		})
	})
}

func TestOpenRCInitSystem(t *testing.T) {
	setupFakeCommand(t)
	openrc := OpenRCInitSystem{}
	serviceName := "kubelet"

	t.Run("ServiceStart", func(t *testing.T) {
		t.Run("should succeed", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", "none")
			err := openrc.ServiceStart(serviceName)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})

		t.Run("should fail when start command fails", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", fmt.Sprintf("rc-service %s start", serviceName))
			err := openrc.ServiceStart(serviceName)
			if err == nil {
				t.Error("expected an error, but got nil")
			}
		})
	})

	t.Run("ServiceStop", func(t *testing.T) {
		t.Run("should succeed", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", "none")
			err := openrc.ServiceStop(serviceName)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})

		t.Run("should fail when stop command fails", func(t *testing.T) {
			t.Setenv("SIMULATE_FAILURE_FOR", fmt.Sprintf("rc-service %s stop", serviceName))
			err := openrc.ServiceStop(serviceName)
			if err == nil {
				t.Error("expected an error, but got nil")
			}
		})
	})
}
