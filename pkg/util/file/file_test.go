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

package file

import (
	"os"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	existingFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(existingFile, []byte("content"), 0600)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	tests := []struct {
		name     string
		filename string
		want     bool
		wantErr  bool
	}{
		{
			name:     "file exists",
			filename: existingFile,
			want:     true,
			wantErr:  false,
		},
		{
			name:     "file does not exist",
			filename: filepath.Join(tempDir, "nonexistent.txt"),
			want:     false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FileExists(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FileExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteAndReadObjectFromYamlFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "pod.yaml")

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}

	// 1. Test writing to a new file (non-existent target path)
	err := WriteObjectToYamlFile(pod, filePath)
	if err != nil {
		t.Fatalf("WriteObjectToYamlFile() error writing new file = %v", err)
	}

	exists, err := FileExists(filePath)
	if err != nil || !exists {
		t.Fatalf("WriteObjectToYamlFile() expected file to exist, exists=%v, err=%v", exists, err)
	}

	// 2. Test reading object back from YAML file
	obj, err := ReadObjectFromYamlFile(filePath)
	if err != nil {
		t.Fatalf("ReadObjectFromYamlFile() error = %v", err)
	}

	readPod, ok := obj.(*v1.Pod)
	if !ok {
		t.Fatalf("ReadObjectFromYamlFile() returned wrong type: %T", obj)
	}
	if readPod.Name != pod.Name || readPod.Namespace != pod.Namespace {
		t.Errorf("ReadObjectFromYamlFile() pod name/namespace mismatch, got %s/%s, want %s/%s",
			readPod.Name, readPod.Namespace, pod.Name, pod.Namespace)
	}

	// 3. Test overwriting an existing file (triggers backupFile and path removal)
	pod.Spec.Containers[0].Image = "nginx:1.21"
	err = WriteObjectToYamlFile(pod, filePath)
	if err != nil {
		t.Fatalf("WriteObjectToYamlFile() error overwriting existing file = %v", err)
	}

	// Clean up backup file from /tmp if created
	bakFile := filepath.Join("/tmp", filepath.Base(filePath))
	if bakExists, _ := FileExists(bakFile); bakExists {
		os.Remove(bakFile)
	}
}

func TestReadObjectFromYamlFile_Errors(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("non-existent file", func(t *testing.T) {
		_, err := ReadObjectFromYamlFile(filepath.Join(tempDir, "missing.yaml"))
		if err == nil {
			t.Errorf("ReadObjectFromYamlFile() expected error for non-existent file")
		}
	})

	t.Run("invalid yaml content", func(t *testing.T) {
		invalidFile := filepath.Join(tempDir, "invalid.yaml")
		if err := os.WriteFile(invalidFile, []byte("invalid yaml content :::"), 0600); err != nil {
			t.Fatalf("failed to write invalid yaml file: %v", err)
		}
		_, err := ReadObjectFromYamlFile(invalidFile)
		if err == nil {
			t.Errorf("ReadObjectFromYamlFile() expected error for invalid YAML content")
		}
	})
}

func TestWriteObjectToYamlFile_Errors(t *testing.T) {
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	t.Run("invalid destination directory", func(t *testing.T) {
		invalidPath := filepath.Join("/nonexistent-directory-xyz/sub", "file.yaml")
		err := WriteObjectToYamlFile(pod, invalidPath)
		if err == nil {
			t.Errorf("WriteObjectToYamlFile() expected error when writing to invalid directory")
		}
	})
}
