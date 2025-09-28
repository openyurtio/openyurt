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

package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_DownloadFile(t *testing.T) {
	// 1. Test successful download
	t.Run("download successfully", func(t *testing.T) {
		// Create a mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "file content")
		}))
		defer server.Close()

		// Use a temporary directory for the downloaded file
		tempDir := t.TempDir()
		savePath := filepath.Join(tempDir, "testfile.txt")

		err := DownloadFile(server.URL, savePath, 3)
		assert.NoError(t, err, "Download should succeed")

		// Verify the file content
		content, err := os.ReadFile(savePath)
		assert.NoError(t, err)
		assert.Equal(t, "file content", string(content))
	})

	// 2. Test server returns an error
	t.Run("server returns non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		savePath := filepath.Join(tempDir, "testfile.txt")

		err := DownloadFile(server.URL, savePath, 2)
		assert.Error(t, err, "Download should fail with non-200 status")

		// Verify the file was not created
		_, err = os.Stat(savePath)
		assert.True(t, os.IsNotExist(err))
	})

	// 3. Test the retry mechanism
	t.Run("download successfully after retry", func(t *testing.T) {
		var requestCount int32 = 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail on the first request, succeed on the second
			if atomic.AddInt32(&requestCount, 1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "success after retry")
		}))
		defer server.Close()

		tempDir := t.TempDir()
		savePath := filepath.Join(tempDir, "testfile.txt")

		err := DownloadFile(server.URL, savePath, 3)
		assert.NoError(t, err, "Download should succeed after retry")

		content, err := os.ReadFile(savePath)
		assert.NoError(t, err)
		assert.Equal(t, "success after retry", string(content))
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "Should have been called twice")
	})

	// 4. Test invalid URL
	t.Run("invalid download url", func(t *testing.T) {
		// Use a URL that will fail to resolve
		err := DownloadFile("http://invalid-url-that-does-not-exist.local", "/tmp/test", 2)
		assert.Error(t, err, "Download should fail for invalid URL")
	})
}

// createTestTarGz is a helper function to create a .tar.gz file for testing.
// files: a map where the key is the filename and the value is the file content.
func createTestTarGz(t *testing.T, files map[string]string) string {
	tarPath := filepath.Join(t.TempDir(), "test.tar.gz")
	tarFile, err := os.Create(tarPath)
	assert.NoError(t, err)
	defer tarFile.Close()

	gw := gzip.NewWriter(tarFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}
		err := tw.WriteHeader(hdr)
		assert.NoError(t, err)
		_, err = io.WriteString(tw, content)
		assert.NoError(t, err)
	}
	return tarPath
}

func Test_Untar(t *testing.T) {
	// 1. Test successful untar
	t.Run("untar successfully", func(t *testing.T) {
		// Prepare test files
		files := map[string]string{
			"file1.txt":      "content1",
			"dir1/file2.txt": "content2",
		}
		tarPath := createTestTarGz(t, files)
		destDir := t.TempDir()

		err := Untar(tarPath, destDir)
		assert.NoError(t, err, "Untar should be successful")

		// Verify extracted files
		content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content1", string(content1))

		content2, err := os.ReadFile(filepath.Join(destDir, "dir1/file2.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content2", string(content2))
	})

	// 2. Test for path traversal vulnerability
	t.Run("should prevent path traversal", func(t *testing.T) {
		files := map[string]string{
			"../../evil.txt": "you are hacked",
		}
		tarPath := createTestTarGz(t, files)
		destDir := t.TempDir()

		err := Untar(tarPath, destDir)
		assert.Error(t, err, "Untar should fail on path traversal")
		assert.Contains(t, err.Error(), "illegal file path", "Error message should indicate security issue")

		// Ensure the malicious file was not created outside the destination
		evilPath := filepath.Join(destDir, "../../evil.txt")
		_, err = os.Stat(evilPath)
		assert.True(t, os.IsNotExist(err), "Malicious file should not be created")
	})

	// 3. Test when the tar file does not exist
	t.Run("tar file does not exist", func(t *testing.T) {
		err := Untar("/path/to/nonexistent/file.tar.gz", t.TempDir())
		assert.Error(t, err, "Untar should fail if source file does not exist")
	})

	// 4. Test invalid tar file format
	t.Run("invalid tar file format", func(t *testing.T) {
		// Create a plain text file, not a tar.gz
		invalidFile := filepath.Join(t.TempDir(), "invalid.txt")
		err := os.WriteFile(invalidFile, []byte("this is not a tar.gz file"), 0644)
		assert.NoError(t, err)

		err = Untar(invalidFile, t.TempDir())
		assert.Error(t, err, "Untar should fail for invalid file format")
		assert.Contains(t, err.Error(), "gzip: invalid header")
	})
}
