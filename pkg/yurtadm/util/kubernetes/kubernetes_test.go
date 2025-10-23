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

package kubernetes

import (
	"reflect"
	"strings"
	"testing"
)

// --- TestIsValidBootstrapToken tests the BootstrapTokenRegexp regex pattern ---
func TestIsValidBootstrapToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "Valid_Token",
			token: "abcdef.1234567890abcdef", // 6-char prefix.16-char suffix
			want:  true,
		},
		{
			name:  "Invalid_Length_Prefix",
			token: "abc.1234567890abcdef", // Prefix length < 6
			want:  false,
		},
		{
			name:  "Invalid_Length_Suffix",
			token: "abcdef.123", // Suffix length < 16
			want:  false,
		},
		{
			name:  "Invalid_Separator",
			token: "abcdef-1234567890abcdef", // Separator is not a dot
			want:  false,
		},
		{
			name:  "Empty_Token",
			token: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidBootstrapToken(tt.token); got != tt.want {
				t.Errorf("IsValidBootstrapToken(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

// --- TestConstructNodeLabels tests the logic for creating node labels string ---
func TestConstructNodeLabels(t *testing.T) {
	// Define a constant Edge Worker label for reference
	const edgeWorkerLabel = "apps.openyurt.io/is-edge-worker"

	tests := []struct {
		name        string
		inputLabels map[string]string
		workingMode string
		expectedMap map[string]string
	}{
		{
			name:        "Cloud_Mode_No_Existing_Label",
			inputLabels: map[string]string{"foo": "bar", "env": "prod"},
			workingMode: "cloud",
			expectedMap: map[string]string{
				"foo":           "bar",
				"env":           "prod",
				edgeWorkerLabel: "false", // Should be automatically added as false
			},
		},
		{
			name:        "Edge_Mode_No_Existing_Label",
			inputLabels: map[string]string{"foo": "bar"},
			workingMode: "edge",
			expectedMap: map[string]string{
				"foo":           "bar",
				edgeWorkerLabel: "true", // Should be automatically added as true
			},
		},
		{
			name:        "Existing_Label_Not_Overwritten",
			inputLabels: map[string]string{edgeWorkerLabel: "custom-value", "zone": "shanghai"},
			workingMode: "cloud", // Existing label should be preserved regardless of workingMode
			expectedMap: map[string]string{
				edgeWorkerLabel: "custom-value",
				"zone":          "shanghai",
			},
		},
		{
			name:        "Empty_Input_Edge_Mode",
			inputLabels: nil,
			workingMode: "edge",
			expectedMap: map[string]string{
				edgeWorkerLabel: "true",
			},
		},
		{
			name:        "Empty_Input_Cloud_Mode",
			inputLabels: map[string]string{},
			workingMode: "cloud",
			expectedMap: map[string]string{
				edgeWorkerLabel: "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultStr := constructNodeLabels(tt.inputLabels, tt.workingMode, edgeWorkerLabel)

			// Parse the result string back to a Map for reliable comparison (due to Map iteration order)
			resultMap := make(map[string]string)
			parts := strings.Split(resultStr, ",")
			for _, part := range parts {
				kv := strings.Split(part, "=")
				if len(kv) == 2 {
					resultMap[kv[0]] = kv[1]
				}
			}

			if !reflect.DeepEqual(resultMap, tt.expectedMap) {
				t.Errorf("constructNodeLabels() result mismatch.\nExpected Map: %v\nActual Map: %v\nActual String: %s",
					tt.expectedMap, resultMap, resultStr)
			}
		})
	}
}
