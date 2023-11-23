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

func TestGetHubServerCertFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		rootDir string
		path    string
	}{
		"use default root dir": {
			rootDir: "",
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), "pki", fmt.Sprintf("%s-server-current.pem", projectinfo.GetHubName())),
		},
		"define root dir": {
			rootDir: "/tmp",
			path:    filepath.Join("/tmp", "pki", fmt.Sprintf("%s-server-current.pem", projectinfo.GetHubName())),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			opt := &options.YurtHubOptions{
				NodeName:    nodeName,
				YurtHubHost: "127.0.0.1",
				RootDir:     tc.rootDir,
			}

			mgr, err := NewYurtHubCertManager(opt, remoteServers)
			if err != nil {
				t.Errorf("failed to new cert manager, %v", err)
			}

			if mgr.GetHubServerCertFile() != tc.path {
				t.Errorf("expect hub server cert file %s, but got %s", tc.path, mgr.GetHubServerCertFile())
			}
		})
	}
}

var (
	joinToken = "123456.abcdef1234567890"
	rootDir   = "/tmp/token/cert"
)

func TestReady(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}

	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
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
		t.Errorf("failed to new yurt cert manager, %v", err)
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
		t.Errorf("failed to new another yurt cert manager, %v", err)
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
