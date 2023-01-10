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
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token/testdata"
)

func Test_removeDirContents(t *testing.T) {
	dir, err := ioutil.TempDir("", "yurthub-test-removeDirContents")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()

	tempFile := dir + "/tmp.txt"
	if err = ioutil.WriteFile(tempFile, nil, 0600); err != nil {
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
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	stopCh := make(chan struct{})
	testcases := map[string]struct {
		rootDir string
		path    string
	}{
		"use default root dir": {
			rootDir: "",
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), fmt.Sprintf("%s.conf", projectinfo.GetHubName())),
		},
		"define root dir": {
			rootDir: "/tmp",
			path:    filepath.Join("/tmp", fmt.Sprintf("%s.conf", projectinfo.GetHubName())),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cfg := &config.YurtHubConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				CertIPs:       certIPs,
				RootDir:       tc.rootDir,
			}

			mgr, err := NewYurtHubCertManager(nil, cfg, stopCh)
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
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	stopCh := make(chan struct{})
	testcases := map[string]struct {
		rootDir string
		path    string
	}{
		"use default root dir": {
			rootDir: "",
			path:    filepath.Join("/var/lib", projectinfo.GetHubName(), "pki", "ca.crt"),
		},
		"define root dir": {
			rootDir: "/tmp",
			path:    filepath.Join("/tmp", "pki", "ca.crt"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cfg := &config.YurtHubConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				CertIPs:       certIPs,
				RootDir:       tc.rootDir,
			}

			mgr, err := NewYurtHubCertManager(nil, cfg, stopCh)
			if err != nil {
				t.Errorf("failed to new cert manager, %v", err)
			}

			if mgr.GetCaFile() != tc.path {
				t.Errorf("expect ca.crt file %s, but got %s", tc.path, mgr.GetCaFile())
			}
		})
	}
}

func TestGetHubServerCertFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	stopCh := make(chan struct{})
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
			cfg := &config.YurtHubConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				CertIPs:       certIPs,
				RootDir:       tc.rootDir,
			}

			mgr, err := NewYurtHubCertManager(nil, cfg, stopCh)
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

func TestUpdateBootstrapConf(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	stopCh := make(chan struct{})
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
			client, err := testdata.CreateCertFakeClient("./testdata")
			if err != nil {
				t.Errorf("failed to create cert fake client, %v", err)
				return
			}

			mgr, err := NewYurtHubCertManager(client, &config.YurtHubConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				CertIPs:       certIPs,
				RootDir:       rootDir,
				JoinToken:     tc.joinToken,
			}, stopCh)
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
	os.RemoveAll(rootDir)
}

func TestReady(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	stopCh := make(chan struct{})

	client, err := testdata.CreateCertFakeClient("./testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}

	mgr, err := NewYurtHubCertManager(client, &config.YurtHubConfiguration{
		NodeName:                 nodeName,
		RemoteServers:            remoteServers,
		CertIPs:                  certIPs,
		RootDir:                  rootDir,
		JoinToken:                joinToken,
		YurtHubCertOrganizations: []string{"yurthub:tenant:foo"},
	}, stopCh)
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
	}

	mgr.Stop()

	// reuse the config and ca file
	t.Logf("go to check the reuse of config and ca file")
	newMgr, err := NewYurtHubCertManager(client, &config.YurtHubConfiguration{
		NodeName:                 nodeName,
		RemoteServers:            remoteServers,
		CertIPs:                  certIPs,
		RootDir:                  rootDir,
		JoinToken:                joinToken,
		YurtHubCertOrganizations: []string{"yurthub:tenant:foo"},
	}, stopCh)
	if err != nil {
		t.Errorf("failed to new another yurt cert manager, %v", err)
		return
	}
	newMgr.Start()
	if !newMgr.Ready() {
		t.Errorf("certificates can not be reused")
	}
	newMgr.Stop()

	os.RemoveAll(rootDir)
}
