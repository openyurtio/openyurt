/*
Copyright 2023 The OpenYurt Authors.

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

package server

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestGetHubServerCertFile(t *testing.T) {
	nodeName := "foo"
	testcases := map[string]struct {
		rootDir string
		path    string
	}{
		"use default root dir": {
			rootDir: filepath.Join("/var/lib", projectinfo.GetHubName(), "pki"),
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), "pki", fmt.Sprintf("%s-server-current.pem", projectinfo.GetHubName())),
		},
		"define root dir": {
			rootDir: "/tmp/pki",
			path:    filepath.Join("/tmp", "pki", fmt.Sprintf("%s-server-current.pem", projectinfo.GetHubName())),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mgr, err := NewHubServerCertificateManager(nil, nil, nodeName, tc.rootDir, nil)
			if err != nil {
				t.Errorf("failed to new cert manager, %v", err)
			}

			if mgr.GetHubServerCertFile() != tc.path {
				t.Errorf("expect hub server cert file %s, but got %s", tc.path, mgr.GetHubServerCertFile())
			}
		})
	}
}
