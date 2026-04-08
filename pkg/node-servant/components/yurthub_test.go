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

package components

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

func TestYurthubOperatorInstall(t *testing.T) {
	oldCheckAndInstallYurthubFunc := checkAndInstallYurthubFunc
	oldCreateYurthubSystemdServiceFunc := createYurthubSystemdServiceFunc
	oldCheckYurthubServiceHealthFunc := checkYurthubServiceHealthFunc
	oldCheckYurthubReadyzFunc := checkYurthubReadyzFunc
	defer func() {
		checkAndInstallYurthubFunc = oldCheckAndInstallYurthubFunc
		createYurthubSystemdServiceFunc = oldCreateYurthubSystemdServiceFunc
		checkYurthubServiceHealthFunc = oldCheckYurthubServiceHealthFunc
		checkYurthubReadyzFunc = oldCheckYurthubReadyzFunc
	}()

	cfg := &yurthubutil.YurthubHostConfig{
		BindAddress: "127.0.0.2",
		NodeName:    "node-1",
		ServerAddr:  "10.0.0.1:6443",
		WorkingMode: "edge",
	}

	var calls []string
	checkAndInstallYurthubFunc = func() error {
		calls = append(calls, "install")
		return nil
	}
	createYurthubSystemdServiceFunc = func(got *yurthubutil.YurthubHostConfig) error {
		if !reflect.DeepEqual(got, cfg) {
			t.Fatalf("unexpected config: %#v", got)
		}
		calls = append(calls, "systemd")
		return nil
	}
	checkYurthubServiceHealthFunc = func(bindAddress string) error {
		if bindAddress != cfg.BindAddress {
			t.Fatalf("unexpected bind address %q", bindAddress)
		}
		calls = append(calls, "healthz")
		return nil
	}
	checkYurthubReadyzFunc = func(bindAddress string) error {
		if bindAddress != cfg.BindAddress {
			t.Fatalf("unexpected bind address %q", bindAddress)
		}
		calls = append(calls, "readyz")
		return nil
	}

	if err := NewYurthubOperator(cfg).Install(); err != nil {
		t.Fatalf("Install() returned error: %v", err)
	}

	wantCalls := []string{"install", "systemd", "healthz", "readyz"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected call order, got=%v, want=%v", calls, wantCalls)
	}
}

func TestYurthubOperatorUnInstall(t *testing.T) {
	oldCleanYurthubHostArtifactsFunc := cleanYurthubHostArtifactsFunc
	defer func() {
		cleanYurthubHostArtifactsFunc = oldCleanYurthubHostArtifactsFunc
	}()

	called := false
	cleanYurthubHostArtifactsFunc = func() error {
		called = true
		return nil
	}

	if err := NewYurthubOperator(nil).UnInstall(); err != nil {
		t.Fatalf("UnInstall() returned error: %v", err)
	}
	if !called {
		t.Fatal("expected CleanYurthubHostArtifacts to be called")
	}
}

func TestInstallEmbeddedYurthub(t *testing.T) {
	oldStatYurthubBinaryFunc := statYurthubBinaryFunc
	oldRemoveYurthubBinaryFunc := removeYurthubBinaryFunc
	oldCopyEmbeddedYurthubBinaryFunc := copyEmbeddedYurthubBinaryFunc
	defer func() {
		statYurthubBinaryFunc = oldStatYurthubBinaryFunc
		removeYurthubBinaryFunc = oldRemoveYurthubBinaryFunc
		copyEmbeddedYurthubBinaryFunc = oldCopyEmbeddedYurthubBinaryFunc
	}()

	t.Run("replace when binary already exists", func(t *testing.T) {
		statYurthubBinaryFunc = func(name string) (os.FileInfo, error) {
			if name != constants.YurthubExecStart {
				t.Fatalf("unexpected stat path %q", name)
			}
			return nil, nil
		}
		removed := false
		removeYurthubBinaryFunc = func(path string) error {
			if path != constants.YurthubExecStart {
				t.Fatalf("unexpected remove path %q", path)
			}
			removed = true
			return nil
		}

		var copied bool
		copyEmbeddedYurthubBinaryFunc = func(src, dest string, mode os.FileMode) error {
			copied = true
			return nil
		}

		if err := installEmbeddedYurthub(); err != nil {
			t.Fatalf("installEmbeddedYurthub() returned error: %v", err)
		}
		if !removed {
			t.Fatal("expected existing binary to be removed")
		}
		if !copied {
			t.Fatal("expected embedded binary to be copied")
		}
	})

	t.Run("copy embedded binary when host binary is missing", func(t *testing.T) {
		statYurthubBinaryFunc = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		removeYurthubBinaryFunc = func(path string) error {
			t.Fatal("remove should not be called when host binary is missing")
			return nil
		}

		var gotSrc, gotDest string
		var gotMode os.FileMode
		copyEmbeddedYurthubBinaryFunc = func(src, dest string, mode os.FileMode) error {
			gotSrc = src
			gotDest = dest
			gotMode = mode
			return nil
		}

		if err := installEmbeddedYurthub(); err != nil {
			t.Fatalf("installEmbeddedYurthub() returned error: %v", err)
		}
		if gotSrc != constants.YurthubEmbeddedPath {
			t.Fatalf("unexpected source, got=%q, want=%q", gotSrc, constants.YurthubEmbeddedPath)
		}
		if gotDest != constants.YurthubExecStart {
			t.Fatalf("unexpected destination, got=%q, want=%q", gotDest, constants.YurthubExecStart)
		}
		if gotMode != 0755 {
			t.Fatalf("unexpected mode, got=%v, want=%v", gotMode, os.FileMode(0755))
		}
	})

	t.Run("return stat error when binary state is unknown", func(t *testing.T) {
		wantErr := errors.New("stat failed")
		statYurthubBinaryFunc = func(name string) (os.FileInfo, error) {
			return nil, wantErr
		}
		removeYurthubBinaryFunc = func(path string) error {
			t.Fatal("remove should not be called when stat fails")
			return nil
		}
		copyEmbeddedYurthubBinaryFunc = func(src, dest string, mode os.FileMode) error {
			t.Fatal("copy should not be called when stat fails")
			return nil
		}

		if err := installEmbeddedYurthub(); !errors.Is(err, wantErr) {
			t.Fatalf("installEmbeddedYurthub() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("return remove error when host binary cleanup fails", func(t *testing.T) {
		wantErr := errors.New("remove failed")
		statYurthubBinaryFunc = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		removeYurthubBinaryFunc = func(path string) error {
			return wantErr
		}
		copyEmbeddedYurthubBinaryFunc = func(src, dest string, mode os.FileMode) error {
			t.Fatal("copy should not be called when remove fails")
			return nil
		}

		if err := installEmbeddedYurthub(); !errors.Is(err, wantErr) {
			t.Fatalf("installEmbeddedYurthub() error = %v, want %v", err, wantErr)
		}
	})
}
