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
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_DownloadFile(t *testing.T) {
	testCase := []struct {
		name     string
		url      string
		retry    int
		expected bool
	}{
		{"invalid download url", "http://1.2.3.4", 2, false},
	}

	for _, tc := range testCase {
		err := DownloadFile(tc.url, "", tc.retry)
		assert.Equal(t, tc.expected, err == nil)
	}
}

func Test_Untar(t *testing.T) {
	testCase := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"invalid tar file", "/tmp", false},
	}

	for _, tc := range testCase {
		err := Untar(tc.name, "")
		assert.Equal(t, tc.expected, err == nil)
	}
}
