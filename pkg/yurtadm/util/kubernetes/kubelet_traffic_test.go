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

package kubernetes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

func TestKubeletYurthubConfigWriteKubeletConfig(t *testing.T) {
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "etc", "kubernetes", "kubelet.conf")
	cfg := &KubeletYurthubConfig{
		KubeconfigFilePath: kubeconfigPath,
		FileMode:           0644,
	}

	if err := cfg.WriteKubeletConfig(); err != nil {
		t.Fatalf("WriteKubeletConfig() returned error: %v", err)
	}

	content, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("failed to read generated kubeconfig: %v", err)
	}
	assert.Equal(t, constants.KubeletConfForNode, string(content))
}

func TestKubeletYurthubConfigAppendAndUndoKubeadmFlags(t *testing.T) {
	tempDir := t.TempDir()
	flagsPath := filepath.Join(tempDir, "kubeadm-flags.env")
	kubeconfigPath := filepath.Join(tempDir, "openyurt", "kubelet.conf")
	initialArgs := `KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///run/containerd/containerd.sock"`
	if err := os.WriteFile(flagsPath, []byte(initialArgs), 0644); err != nil {
		t.Fatalf("failed to prepare kubeadm-flags.env: %v", err)
	}

	cfg := &KubeletYurthubConfig{
		KubeconfigFilePath:   kubeconfigPath,
		KubeadmFlagsFilePath: flagsPath,
		FileMode:             0644,
	}

	if err := cfg.AppendKubeadmFlags(); err != nil {
		t.Fatalf("AppendKubeadmFlags() returned error: %v", err)
	}

	content, err := os.ReadFile(flagsPath)
	if err != nil {
		t.Fatalf("failed to read kubeadm-flags.env: %v", err)
	}

	appendSetting := cfg.appendSetting()
	assert.Contains(t, string(content), appendSetting)

	if err := cfg.AppendKubeadmFlags(); err != nil {
		t.Fatalf("AppendKubeadmFlags() should be idempotent, got error: %v", err)
	}

	content, err = os.ReadFile(flagsPath)
	if err != nil {
		t.Fatalf("failed to reread kubeadm-flags.env: %v", err)
	}
	assert.Equal(t, 1, strings.Count(string(content), appendSetting))

	if err := cfg.UndoAppendKubeadmFlags(); err != nil {
		t.Fatalf("UndoAppendKubeadmFlags() returned error: %v", err)
	}

	content, err = os.ReadFile(flagsPath)
	if err != nil {
		t.Fatalf("failed to reread kubeadm-flags.env after undo: %v", err)
	}
	assert.NotContains(t, string(content), appendSetting)
}

func TestKubeletYurthubConfigRedirectAndUndoTraffic(t *testing.T) {
	tempDir := t.TempDir()
	flagsPath := filepath.Join(tempDir, "kubeadm-flags.env")
	kubeconfigPath := filepath.Join(tempDir, "var", "lib", "openyurt", "kubelet.conf")
	initialArgs := `KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///run/containerd/containerd.sock"`
	if err := os.WriteFile(flagsPath, []byte(initialArgs), 0644); err != nil {
		t.Fatalf("failed to prepare kubeadm-flags.env: %v", err)
	}

	oldExecCommand := kubeletExecCommand
	oldExec := kubeletExec
	defer func() {
		kubeletExecCommand = oldExecCommand
		kubeletExec = oldExec
	}()

	kubeletExecCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}
	kubeletExec = func(cmd *exec.Cmd) error {
		return cmd.Run()
	}

	cfg := &KubeletYurthubConfig{
		KubeconfigFilePath:   kubeconfigPath,
		KubeadmFlagsFilePath: flagsPath,
		FileMode:             0644,
	}

	if err := cfg.RedirectTrafficToYurthub(); err != nil {
		t.Fatalf("RedirectTrafficToYurthub() returned error: %v", err)
	}

	_, err := os.Stat(kubeconfigPath)
	assert.NoError(t, err)

	content, err := os.ReadFile(flagsPath)
	if err != nil {
		t.Fatalf("failed to read kubeadm-flags.env after redirect: %v", err)
	}
	assert.Contains(t, string(content), cfg.appendSetting())

	if err := cfg.UndoRedirectTrafficToYurthub(); err != nil {
		t.Fatalf("UndoRedirectTrafficToYurthub() returned error: %v", err)
	}

	_, err = os.Stat(kubeconfigPath)
	assert.True(t, os.IsNotExist(err))

	content, err = os.ReadFile(flagsPath)
	if err != nil {
		t.Fatalf("failed to read kubeadm-flags.env after undo: %v", err)
	}
	assert.NotContains(t, string(content), cfg.appendSetting())
}
