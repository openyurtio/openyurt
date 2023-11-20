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
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/ca"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func TestGetHubConfFile(t *testing.T) {
	nodeName := "foo"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		workDir string
		path    string
	}{
		"use tmp lib dir": {
			workDir: filepath.Join("/tmp/lib", projectinfo.GetHubName()),
			path:    filepath.Join("/tmp/lib", projectinfo.GetHubName(), fmt.Sprintf("%s.conf", projectinfo.GetHubName())),
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

			mgr, err := NewYurtHubClientCertManager(cfg, nil)
			if err != nil {
				t.Errorf("could not new cert manager, %v", err)
			}

			if mgr.GetHubConfFile() != tc.path {
				t.Errorf("expect hub conf file %s, but got %s", tc.path, mgr.GetHubConfFile())
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
		},
	}

	client, err := testdata.CreateCertFakeClient("../../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	caMgr, err := ca.NewCAManager(client, certificate.TokenBoostrapMode, workDir, "", joinToken, remoteServers, []string{})
	if err != nil {
		t.Errorf("could not new ca manager, %v", err)
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mgr, err := NewYurtHubClientCertManager(&ClientCertificateManagerConfiguration{
				NodeName:      nodeName,
				RemoteServers: remoteServers,
				WorkDir:       workDir,
				JoinToken:     tc.joinToken,
				Client:        client,
			}, caMgr)
			if err != nil {
				t.Errorf("could not new yurt cert manager, %v", err)
				return
			}

			resultErr := mgr.UpdateBootstrapConf(tc.joinToken)
			if resultErr != nil {
				t.Errorf("expect no err, but got %v", resultErr)
			}
			mgr.Stop()
		})
	}
	os.RemoveAll(workDir)
}
