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

package yurthub

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

type hostConfigJoinData struct {
	mockYurtJoinData
	binaryURL string
}

func (d *hostConfigJoinData) YurtHubBinaryUrl() string {
	return d.binaryURL
}

func TestNewYurthubHostConfigFromJoinData(t *testing.T) {
	data := &hostConfigJoinData{
		mockYurtJoinData: mockYurtJoinData{
			serverAddr: "10.0.0.1:6443,https://10.0.0.2:6443",
			nodeRegistration: &joindata.NodeRegistration{
				Name:         "node-1",
				NodePoolName: "pool-a",
				WorkingMode:  constants.EdgeNode,
			},
			namespace: "custom-ns",
		},
		binaryURL: "https://example.com/yurthub",
	}

	cfg := NewYurthubHostConfigFromJoinData(data)
	assert.Equal(t, constants.DefaultYurtHubServerAddr, cfg.BindAddress)
	assert.Equal(t, data.binaryURL, cfg.BinaryURL)
	assert.Equal(t, yurthubBootstrapConfigPath, cfg.BootstrapFile)
	assert.Equal(t, "custom-ns", cfg.Namespace)
	assert.Equal(t, "node-1", cfg.NodeName)
	assert.Equal(t, "pool-a", cfg.NodePoolName)
	assert.Equal(t, "10.0.0.1:6443,https://10.0.0.2:6443", cfg.ServerAddr)
	assert.Equal(t, constants.YurthubVersion, cfg.Version)
	assert.Equal(t, constants.EdgeNode, cfg.WorkingMode)
	assert.Equal(t, "https://10.0.0.1:6443,https://10.0.0.2:6443", cfg.normalizedServerAddr())
}

func TestYurthubHostConfigBootstrapArgs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *YurthubHostConfig
		want string
	}{
		{
			name: "bootstrap mode only",
			cfg: &YurthubHostConfig{
				BootstrapMode: "kubeletcertificate",
			},
			want: "--bootstrap-mode=kubeletcertificate",
		},
		{
			name: "bootstrap file only",
			cfg: &YurthubHostConfig{
				BootstrapFile: "/var/lib/yurthub/bootstrap.conf",
			},
			want: "--bootstrap-file=/var/lib/yurthub/bootstrap.conf",
		},
		{
			name: "bootstrap mode and file",
			cfg: &YurthubHostConfig{
				BootstrapMode: "kubeletcertificate",
				BootstrapFile: "/var/lib/yurthub/bootstrap.conf",
			},
			want: "--bootstrap-mode=kubeletcertificate --bootstrap-file=/var/lib/yurthub/bootstrap.conf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.bootstrapArgs())
		})
	}
}

func TestSetYurthubUnitServiceWithBootstrapMode(t *testing.T) {
	useTempYurthubHostPaths(t)

	cfg := &YurthubHostConfig{
		BootstrapMode: "kubeletcertificate",
		Namespace:     "kube-system",
		NodeName:      "node-1",
		NodePoolName:  "pool-a",
		ServerAddr:    "10.0.0.1:6443,https://10.0.0.2:6443",
		WorkingMode:   constants.EdgeNode,
	}

	if err := setYurthubUnitServiceWithConfig(filepath.Dir(yurthubServiceConfFilePath), cfg); err != nil {
		t.Fatalf("setYurthubUnitServiceWithConfig() returned error: %v", err)
	}

	content, err := os.ReadFile(yurthubServiceConfFilePath)
	if err != nil {
		t.Fatalf("failed to read generated systemd drop-in: %v", err)
	}

	unitContent := string(content)
	assert.Contains(t, unitContent, `Environment="YURTHUB_BOOTSTRAP_ARGS=--bootstrap-mode=kubeletcertificate"`)
	assert.NotContains(t, unitContent, "--bootstrap-file=")
	assert.Contains(t, unitContent, "--nodepool-name=pool-a")
	assert.Contains(t, unitContent, "--server-addr=https://10.0.0.1:6443,https://10.0.0.2:6443")
	assert.True(t, strings.Contains(unitContent, "--working-mode=edge"))
}

func TestCleanYurthubHostArtifacts(t *testing.T) {
	tempDir := useTempYurthubHostPaths(t)
	oldExecCommand := execCommand
	defer func() {
		execCommand = oldExecCommand
	}()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", "ok")
	}

	paths := []string{
		yurthubExecStartPath,
		yurthubServiceFilePath,
		yurthubServiceConfFilePath,
		yurthubBootstrapConfigPath,
		filepath.Join(yurthubWorkDirPath, "cache"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to prepare %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("dummy"), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	if err := CleanYurthubHostArtifacts(); err != nil {
		t.Fatalf("CleanYurthubHostArtifacts() returned error: %v", err)
	}

	for _, path := range []string{
		yurthubExecStartPath,
		yurthubServiceFilePath,
		filepath.Dir(yurthubServiceConfFilePath),
		yurthubWorkDirPath,
	} {
		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "expected %s to be removed", path)
	}
	_, err := os.Stat(filepath.Join(tempDir, "var", "lib", "yurthub", "bootstrap-hub.conf"))
	assert.True(t, os.IsNotExist(err))
}
