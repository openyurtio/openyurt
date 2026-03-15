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
	"reflect"
	"testing"

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
		Version:     "v1.6.1",
	}

	var calls []string
	checkAndInstallYurthubFunc = func(got *yurthubutil.YurthubHostConfig) error {
		if !reflect.DeepEqual(got, cfg) {
			t.Fatalf("unexpected config: %#v", got)
		}
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
