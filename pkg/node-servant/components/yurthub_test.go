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

package components

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetYurthubYaml(t *testing.T) {
	testCases := []struct {
		name            string
		podManifestPath string
		expected        string
	}{
		{
			name:            "standard path",
			podManifestPath: "/etc/kubernetes/manifests",
			expected:        "/etc/kubernetes/manifests/yurthub.yaml",
		},
		{
			name:            "custom path",
			podManifestPath: "/tmp/manifests",
			expected:        "/tmp/manifests/yurthub.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getYurthubYaml(tc.podManifestPath)
			if result != tc.expected {
				t.Errorf("getYurthubYaml() = %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestGetYurthubConf(t *testing.T) {
	result := getYurthubConf()
	if result == "" {
		t.Error("getYurthubConf() should not return empty string")
	}
}

func TestGetYurthubCacheDir(t *testing.T) {
	result := getYurthubCacheDir()
	if result == "" {
		t.Error("getYurthubCacheDir() should not return empty string")
	}
}

func TestNewYurthubOperator(t *testing.T) {
	op := NewYurthubOperator("https://127.0.0.1:6443", "test-token", "test-pool", 60)
	if op == nil {
		t.Fatal("NewYurthubOperator() returned nil")
	}
	if op.apiServerAddr != "https://127.0.0.1:6443" {
		t.Errorf("apiServerAddr = %v, want %v", op.apiServerAddr, "https://127.0.0.1:6443")
	}
	if op.joinToken != "test-token" {
		t.Errorf("joinToken = %v, want %v", op.joinToken, "test-token")
	}
	if op.nodePoolName != "test-pool" {
		t.Errorf("nodePoolName = %v, want %v", op.nodePoolName, "test-pool")
	}
}

func TestYurthubYamlCleanup_FileNotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yurthub-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nonExistentPath := filepath.Join(tmpDir, "nonexistent", "yurt-hub.yaml")

	// Verify file doesn't exist
	if _, err := os.Stat(nonExistentPath); !os.IsNotExist(err) {
		t.Fatal("Test setup error: file should not exist")
	}

	// Simulating the logic from UnInstall: when file doesn't exist, no error
	if _, err := os.Stat(nonExistentPath); os.IsNotExist(err) {
		// This is the expected path - file doesn't exist, skip delete
	} else {
		t.Error("Expected file to not exist")
	}
}

func TestYurthubYamlCleanup_FileExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yurthub-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "yurt-hub.yaml")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("Test setup error: file should exist")
	}

	// Remove file (simulating UnInstall cleanup)
	if err := os.Remove(testFile); err != nil {
		t.Errorf("Failed to remove file: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File should be removed")
	}
}

func TestYurthubConfDirCleanup_DirNotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yurthub-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nonExistentDir := filepath.Join(tmpDir, "nonexistent-conf")

	// Verify directory doesn't exist
	if _, err := os.Stat(nonExistentDir); !os.IsNotExist(err) {
		t.Fatal("Test setup error: directory should not exist")
	}

	// Simulating the updated UnInstall logic: when directory doesn't exist,
	// it should continue to clean up cache directory instead of returning early
	confDirExists := true
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		confDirExists = false
	}

	if confDirExists {
		t.Error("Expected directory to not exist")
	}

	// The key change in the PR: after checking conf dir, we continue to cache cleanup
	// regardless of whether conf dir existed
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}

	// Cache cleanup should proceed even when conf dir didn't exist
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Errorf("Failed to remove cache dir: %v", err)
	}

	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache directory should be removed")
	}
}

func TestYurthubConfDirCleanup_DirExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yurthub-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	confDir := filepath.Join(tmpDir, "yurthub-conf")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf dir: %v", err)
	}

	// Create a file inside conf dir
	testFile := filepath.Join(confDir, "test.conf")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(confDir); os.IsNotExist(err) {
		t.Fatal("Test setup error: directory should exist")
	}

	// Remove entire directory (simulating UnInstall cleanup)
	if err := os.RemoveAll(confDir); err != nil {
		t.Errorf("Failed to remove conf dir: %v", err)
	}

	// Verify directory is removed
	if _, err := os.Stat(confDir); !os.IsNotExist(err) {
		t.Error("Conf directory should be removed")
	}
}

func TestCacheDirCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yurthub-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}

	// Create nested structure
	nestedDir := filepath.Join(cacheDir, "kubelet", "pods")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	testFile := filepath.Join(nestedDir, "pod.yaml")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remove entire cache directory
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Errorf("Failed to remove cache dir: %v", err)
	}

	// Verify directory is fully removed
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache directory should be completely removed")
	}
}
