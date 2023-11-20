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

package manager

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

var (
	joinToken = "123456.abcdef1234567890"
	rootDir   = "/tmp/token/cert"
)

func TestGetHubServerCertFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		rootDir string
		path    string
	}{
		"define root dir": {
			rootDir: "/tmp",
			path:    filepath.Join("/tmp", "pki", fmt.Sprintf("%s-server-current.pem", projectinfo.GetHubName())),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			client, err := testdata.CreateCertFakeClient("../testdata")
			if err != nil {
				t.Errorf("could not create cert fake client, %v", err)
				return
			}
			opt := &options.YurtHubOptions{
				NodeName:      nodeName,
				YurtHubHost:   "127.0.0.1",
				RootDir:       tc.rootDir,
				JoinToken:     joinToken,
				ClientForTest: client,
			}

			mgr, err := NewYurtHubCertManager(opt, remoteServers)
			if err != nil {
				t.Errorf("could not new cert manager, %v", err)
			}

			if mgr != nil && mgr.GetHubServerCertFile() != tc.path {
				t.Errorf("expect hub server cert file %s, but got %s", tc.path, mgr.GetHubServerCertFile())
			}
		})
	}
}

func TestReady(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}

	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	mgr, err := NewYurtHubCertManager(&options.YurtHubOptions{
		NodeName:                 nodeName,
		YurtHubHost:              "127.0.0.1",
		RootDir:                  rootDir,
		JoinToken:                joinToken,
		YurtHubCertOrganizations: []string{"yurthub:tenant:foo"},
		ClientForTest:            client,
	}, remoteServers)
	if err != nil {
		t.Errorf("could not new yurt cert manager, %v", err)
		return
	}
	mgr.Start()

	err = wait.PollImmediate(2*time.Second, 1*time.Minute, func() (done bool, err error) {
		if mgr.Ready() {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Errorf("certificates are not ready, %v", err)
		mgr.Stop()
		return
	}

	mgr.Stop()

	// reuse the config and ca file
	t.Logf("go to check the reuse of config and ca file")
	newMgr, err := NewYurtHubCertManager(&options.YurtHubOptions{
		NodeName:                 nodeName,
		YurtHubHost:              "127.0.0.1",
		RootDir:                  rootDir,
		JoinToken:                joinToken,
		YurtHubCertOrganizations: []string{"yurthub:tenant:foo"},
		ClientForTest:            client,
	}, remoteServers)
	if err != nil {
		t.Errorf("could not new another yurt cert manager, %v", err)
		return
	}
	newMgr.Start()
	err = wait.PollImmediate(2*time.Second, 1*time.Minute, func() (done bool, err error) {
		if mgr.Ready() {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("certificates are not reused, %v", err)
	}
	newMgr.Stop()

	os.RemoveAll(rootDir)
}

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
