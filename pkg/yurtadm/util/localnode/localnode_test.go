/*
Copyright 2025 The OpenYurt Authors.

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

package localnode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockIptables is a mock implementation of the iptablesInterface.
type mockIptables struct {
	// key format is "table/chain"
	chains map[string][]string
	// fields for simulating errors
	chainExistsErr error
	listErr        error
}

func (m *mockIptables) ChainExists(table, chain string) (bool, error) {
	if m.chainExistsErr != nil {
		return false, m.chainExistsErr
	}
	_, exists := m.chains[table+"/"+chain]
	return exists, nil
}
func (m *mockIptables) List(table, chain string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	rules, exists := m.chains[table+"/"+chain]
	if !exists {
		return nil, fmt.Errorf("chain %s does not exist", chain)
	}
	return rules, nil
}

// TestDownloadAndInstallYurthub tests the download and installation logic.
func TestDownloadAndInstallYurthub(t *testing.T) {
	// --- Mock all external function calls ---
	originalDownload := utilDownloadFile
	originalUntar := utilUntar
	originalCopy := edgenodeCopyFile
	originalRemoveAll := osRemoveAll
	t.Cleanup(func() { // Ensure original functions are restored after the test.
		utilDownloadFile = originalDownload
		utilUntar = originalUntar
		edgenodeCopyFile = originalCopy
		osRemoveAll = originalRemoveAll
	})

	// Default mock behavior: all operations succeed.
	utilDownloadFile = func(url, savePath string, retries int) error { return nil }
	edgenodeCopyFile = func(src, dst string, perm os.FileMode) error { return nil }
	// In tests, we don't want to actually delete the directory, so replace it with an empty function.
	osRemoveAll = func(path string) error { return nil }

	t.Run("should find and install binary successfully", func(t *testing.T) {
		// Create an independent temporary directory for each subtest.
		tmpDir := t.TempDir()
		originalYurthubTmpDir := yurthubTmpDir
		yurthubTmpDir = tmpDir
		t.Cleanup(func() {
			yurthubTmpDir = originalYurthubTmpDir
		})

		var copySrc, copyDst string
		// Mock untar to create a fake yurthub binary inside a subdirectory.
		utilUntar = func(src, dst string) error {
			subDir := filepath.Join(dst, "yurthub-v1.0")
			if err := os.Mkdir(subDir, 0755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(subDir, "yurthub"), []byte("fake binary"), 0755)
		}
		// Mock file copy to record the source and destination paths.
		edgenodeCopyFile = func(src, dst string, perm os.FileMode) error {
			copySrc = src
			copyDst = dst
			return nil
		}

		err := DownloadAndInstallYurthub("http://fake.url/yurthub.tar.gz")

		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, "yurthub-v1.0", "yurthub"), copySrc, "Source path for copy is incorrect")
		assert.Equal(t, "/usr/bin/yurthub", copyDst, "Destination path for copy is incorrect")
	})

	t.Run("should return an error when download fails", func(t *testing.T) {
		// Create an independent temporary directory for each subtest.
		tmpDir := t.TempDir()
		originalYurthubTmpDir := yurthubTmpDir
		yurthubTmpDir = tmpDir
		t.Cleanup(func() {
			yurthubTmpDir = originalYurthubTmpDir
		})

		expectedErr := errors.New("network error")
		utilDownloadFile = func(url, savePath string, retries int) error {
			return expectedErr
		}

		err := DownloadAndInstallYurthub("http://fake.url/yurthub.tar.gz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedErr.Error(), "Error message should contain the underlying download error")
	})

	t.Run("should return an error when binary is not found after untar", func(t *testing.T) {
		// Create an independent temporary directory for each subtest.
		tmpDir := t.TempDir()
		originalYurthubTmpDir := yurthubTmpDir
		yurthubTmpDir = tmpDir
		t.Cleanup(func() {
			yurthubTmpDir = originalYurthubTmpDir
		})

		// Restore successful download behavior.
		utilDownloadFile = func(url, savePath string, retries int) error { return nil }
		// Mock successful untar, but without a yurthub file inside.
		utilUntar = func(src, dst string) error {
			// Write an unrelated file in the clean directory.
			return os.WriteFile(filepath.Join(dst, "not-yurthub"), []byte("fake binary"), 0755)
		}

		err := DownloadAndInstallYurthub("http://fake.url/yurthub.tar.gz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no binary file named 'yurthub' found")
	})
}

// TestCheckIptablesChainAndRulesExists tests the iptables chain check function.
func TestCheckIptablesChainAndRulesExists(t *testing.T) {
	// Save the original function and restore it after the test.
	originalNewIptables := newIptables
	t.Cleanup(func() { newIptables = originalNewIptables })

	t.Run("should return true when chain exists and has rules", func(t *testing.T) {
		mock := &mockIptables{chains: map[string][]string{
			"nat/TEST-CHAIN": {"-N TEST-CHAIN", "-A TEST-CHAIN ..."}, // exists and has rules
		}}
		newIptables = func() (iptablesInterface, error) { return mock, nil }

		exists, err := CheckIptablesChainAndRulesExists("nat", "TEST-CHAIN")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("should return false when chain exists but has no rules", func(t *testing.T) {
		mock := &mockIptables{chains: map[string][]string{
			"nat/NO-RULES": {"-N NO-RULES"}, // exists but only has the definition, no effective rules
		}}
		newIptables = func() (iptablesInterface, error) { return mock, nil }

		exists, err := CheckIptablesChainAndRulesExists("nat", "NO-RULES")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("should return false when chain does not exist", func(t *testing.T) {
		mock := &mockIptables{chains: map[string][]string{}}
		newIptables = func() (iptablesInterface, error) { return mock, nil }

		exists, err := CheckIptablesChainAndRulesExists("nat", "NON-EXISTENT-CHAIN")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

// TestWaitForIptablesChainReadyWithTimeout tests the wait-with-timeout function.
func TestWaitForIptablesChainReadyWithTimeout(t *testing.T) {
	// Save the original function and restore it after the test.
	originalCheckFunc := checkChainFunc
	t.Cleanup(func() { checkChainFunc = originalCheckFunc })

	t.Run("should return successfully when ready before timeout", func(t *testing.T) {
		callCount := 0
		// Mock the check function: returns false for the first two calls, then true.
		mockCheck := func(table, chain string) (bool, error) {
			callCount++
			if callCount < 3 {
				return false, nil
			}
			return true, nil
		}
		checkChainFunc = mockCheck // perform monkey patch

		// Use very short intervals to make the test run quickly.
		err := WaitForIptablesChainReadyWithTimeout("nat", "test", 10*time.Millisecond, 100*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("should return a timeout error when not ready after timeout", func(t *testing.T) {
		// Mock a check function that always returns false.
		mockCheck := func(table, chain string) (bool, error) {
			return false, nil
		}
		checkChainFunc = mockCheck // perform monkey patch

		err := WaitForIptablesChainReadyWithTimeout("nat", "test", 10*time.Millisecond, 50*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timed out", "Error message should contain 'timed out'")
	})
}

// TestStopYurthubService tests the stop service function.
func TestStopYurthubService(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{
			"normal",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StopYurthubService()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSetYurthubService tests the set service function.
func TestSetYurthubService(t *testing.T) {
	tests := []struct {
		name                 string
		hostControlPlaneAddr string
		serverAddr           string
		nodeName             string
		expectErr            bool
	}{
		{
			"normal",
			"192.168.0.1:8443",
			"192.168.0.1:12345",
			"node",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetYurthubService(tt.hostControlPlaneAddr, tt.serverAddr, tt.nodeName)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEnableYurthubService tests the enable service function.
func TestEnableYurthubService(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{
			"normal",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnableYurthubService()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestStartYurthubService tests the start service function.
func TestStartYurthubService(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{
			"normal",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StartYurthubService()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCheckYurthubStatus tests the check service status function.
func TestCheckYurthubStatus(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{
			"normal",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CheckYurthubStatus()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDownloadAndDeployYurthubInSystemd(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{
			"normal",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DownloadAndDeployYurthubInSystemd("192.168.0.1:8443", "192.168.0.1:12345", "https://yurthub.com/yurthub.tar.gz", "node")
			if tt.expectErr {
				assert.Error(t, err)
			}
		})
	}
}
