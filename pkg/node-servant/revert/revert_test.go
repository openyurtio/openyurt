/*
Copyright 2026 The OpenYurt Authors.

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

package revert

import (
	"errors"
	"reflect"
	"testing"
)

func TestNodeReverterDo(t *testing.T) {
	oldUndoKubeletRedirectFunc := undoKubeletRedirectFunc
	oldUninstallYurthubFunc := uninstallYurthubFunc
	oldRestartContainersFunc := restartContainersFunc
	defer func() {
		undoKubeletRedirectFunc = oldUndoKubeletRedirectFunc
		uninstallYurthubFunc = oldUninstallYurthubFunc
		restartContainersFunc = oldRestartContainersFunc
	}()

	var calls []string
	undoKubeletRedirectFunc = func(openyurtDir string) error {
		if openyurtDir != "/var/lib/openyurt" {
			t.Fatalf("unexpected openyurt dir %q", openyurtDir)
		}
		calls = append(calls, "revert-kubelet")
		return nil
	}
	uninstallYurthubFunc = func() error {
		calls = append(calls, "uninstall-yurthub")
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		if nodeName != "node-a" {
			t.Fatalf("unexpected node name %q", nodeName)
		}
		calls = append(calls, "restart-containers")
		return nil
	}

	reverter := NewReverterWithOptions(&Options{nodeName: "node-a", openyurtDir: "/var/lib/openyurt"})
	if err := reverter.Do(); err != nil {
		t.Fatalf("Do() returned error: %v", err)
	}

	wantCalls := []string{"revert-kubelet", "restart-containers", "uninstall-yurthub"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected call order, got=%v, want=%v", calls, wantCalls)
	}
}

func TestNodeReverterStopsWhenKubeletRevertFails(t *testing.T) {
	oldUndoKubeletRedirectFunc := undoKubeletRedirectFunc
	oldUninstallYurthubFunc := uninstallYurthubFunc
	oldRestartContainersFunc := restartContainersFunc
	defer func() {
		undoKubeletRedirectFunc = oldUndoKubeletRedirectFunc
		uninstallYurthubFunc = oldUninstallYurthubFunc
		restartContainersFunc = oldRestartContainersFunc
	}()

	expectedErr := errors.New("revert failed")
	uninstallCalled := false
	restartCalled := false
	undoKubeletRedirectFunc = func(openyurtDir string) error {
		return expectedErr
	}
	uninstallYurthubFunc = func() error {
		uninstallCalled = true
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		restartCalled = true
		return nil
	}

	reverter := NewReverterWithOptions(&Options{nodeName: "node-a", openyurtDir: "/var/lib/openyurt"})
	err := reverter.Do()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("unexpected error, got=%v, want=%v", err, expectedErr)
	}
	if uninstallCalled {
		t.Fatal("expected yurthub uninstall to be skipped after kubelet revert failure")
	}
	if restartCalled {
		t.Fatal("expected container restart to be skipped after kubelet revert failure")
	}
}
