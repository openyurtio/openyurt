/*
Copyright 2020 The OpenYurt Authors.

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

package token

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func Test_removeDirContents(t *testing.T) {
	dir, err := os.MkdirTemp("", "yurthub-test-removeDirContents")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()

	tempFile := dir + "/tmp.txt"
	if err = os.WriteFile(tempFile, nil, 0600); err != nil {
		t.Fatalf("Unable to create the test file %q: %v", tempFile, err)
	}

	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		file    string
		wantErr bool
	}{
		{"no input dir", args{}, "", true},
		{"input dir exist", args{dir}, tempFile, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err = removeDirContents(tt.args.dir); (err != nil) != tt.wantErr {
				t.Errorf("removeDirContents() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.file != "" {
				if _, err = os.Stat(tt.file); err == nil || !os.IsNotExist(err) {
					t.Errorf("after remote dir content, no file should exist. file:%s", tt.file)
				}
			}
		})
	}
}

func TestGetHubConfFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		workDir string
		path    string
	}{
		"use default root dir": {
			workDir: filepath.Join("/var/lib", projectinfo.GetHubName()),
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), fmt.Sprintf("%s.conf", projectinfo.GetHubName())),
		},
		"define root dir": {
			workDir: "/tmp",
			path:    filepath.Join("/tmp", fmt.Sprintf("%s.conf", projectinfo.GetHubName())),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cfg := &ClientCertificateManagerConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				WorkDir:       tc.workDir,
			}

			mgr, err := NewYurtHubClientCertManager(cfg)
			if err != nil {
				t.Errorf("failed to new cert manager, %v", err)
			}

			if mgr.GetHubConfFile() != tc.path {
				t.Errorf("expect hub conf file %s, but got %s", tc.path, mgr.GetHubConfFile())
			}
		})
	}
}

func TestGetCaFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		workDir string
		path    string
	}{
		"use default root dir": {
			workDir: filepath.Join("/var/lib", projectinfo.GetHubName()),
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), "pki", "ca.crt"),
		},
		"define root dir": {
			workDir: "/tmp",
			path:    filepath.Join("/tmp", "pki", "ca.crt"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cfg := &ClientCertificateManagerConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				WorkDir:       tc.workDir,
			}

			mgr, err := NewYurtHubClientCertManager(cfg)
			if err != nil {
				t.Errorf("failed to new cert manager, %v", err)
			}

			if mgr.GetCaFile() != tc.path {
				t.Errorf("expect ca.crt file %s, but got %s", tc.path, mgr.GetCaFile())
			}
		})
	}
}

func TestUpdateBootstrapConf(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	workDir := "/tmp/token/cert"
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		joinToken string
		err       error
	}{
		"normal update": {
			joinToken: joinToken,
			err:       nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			client, err := testdata.CreateCertFakeClient("../testdata")
			if err != nil {
				t.Errorf("failed to create cert fake client, %v", err)
				return
			}

			mgr, err := NewYurtHubClientCertManager(&ClientCertificateManagerConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				WorkDir:       workDir,
				JoinToken:     tc.joinToken,
				Client:        client,
			})
			if err != nil {
				t.Errorf("failed to new yurt cert manager, %v", err)
				return
			}

			resultErr := mgr.UpdateBootstrapConf(tc.joinToken)
			if resultErr != tc.err {
				t.Errorf("expect err is %v, but got %v", tc.err, resultErr)
			}
			mgr.Stop()
		})
	}
	os.RemoveAll(workDir)
}
