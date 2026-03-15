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
	"path/filepath"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

type fakeKubeletTrafficManager struct {
	redirectErr error
	undoErr     error
}

func (m *fakeKubeletTrafficManager) RedirectTrafficToYurthub() error {
	return m.redirectErr
}

func (m *fakeKubeletTrafficManager) UndoRedirectTrafficToYurthub() error {
	return m.undoErr
}

func TestKubeletOperatorRedirectTrafficToYurtHub(t *testing.T) {
	operator := NewKubeletOperator("/var/lib/openyurt")
	oldFactory := newKubeletTrafficManager
	defer func() {
		newKubeletTrafficManager = oldFactory
	}()

	var gotKubeconfigPath string
	var gotKubeadmFlagsFilePath string
	newKubeletTrafficManager = func(kubeconfigFilePath, kubeadmFlagsFilePath string) kubeletTrafficManager {
		gotKubeconfigPath = kubeconfigFilePath
		gotKubeadmFlagsFilePath = kubeadmFlagsFilePath
		return &fakeKubeletTrafficManager{}
	}

	if err := operator.RedirectTrafficToYurtHub(); err != nil {
		t.Fatalf("RedirectTrafficToYurtHub() returned error: %v", err)
	}

	expectedKubeconfigPath := filepath.Join("/var/lib/openyurt", constants.KubeletKubeConfigFileName)
	if gotKubeconfigPath != expectedKubeconfigPath {
		t.Fatalf("unexpected kubeconfig path, want %s, got %s", expectedKubeconfigPath, gotKubeconfigPath)
	}
	if gotKubeadmFlagsFilePath != constants.KubeadmFlagsEnvFilePath {
		t.Fatalf("unexpected kubeadm-flags path, want %s, got %s", constants.KubeadmFlagsEnvFilePath, gotKubeadmFlagsFilePath)
	}
}

func TestKubeletOperatorUndoRedirectTrafficToYurtHub(t *testing.T) {
	operator := NewKubeletOperator("/var/lib/openyurt")
	oldFactory := newKubeletTrafficManager
	defer func() {
		newKubeletTrafficManager = oldFactory
	}()

	expectedErr := errors.New("undo failed")
	newKubeletTrafficManager = func(kubeconfigFilePath, kubeadmFlagsFilePath string) kubeletTrafficManager {
		return &fakeKubeletTrafficManager{undoErr: expectedErr}
	}

	err := operator.UndoRedirectTrafficToYurtHub()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("UndoRedirectTrafficToYurtHub() error = %v, want %v", err, expectedErr)
	}
}
