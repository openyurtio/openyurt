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

package ca

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
)

func TestNewCAManager(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	mgr, err := NewCAManager(client, certificate.TokenBoostrapMode, "/tmp", "", joinToken, remoteServers, []string{})
	if err != nil {
		t.Errorf("could not new cert manager, %v", err)
	}

	if !bytes.Equal(mgr.GetCAData(), testdata.GetCAData()) {
		t.Errorf("expect ca data %v, but got %v", testdata.GetCAData(), mgr.GetCAData())
	}

	// restart to ca manager, and check ca data is ready or not
	newMgr, err := NewCAManager(client, certificate.TokenBoostrapMode, "/tmp", "", joinToken, remoteServers, []string{})
	if err != nil {
		t.Errorf("could not new cert manager, %v", err)
	}

	if !bytes.Equal(newMgr.GetCAData(), testdata.GetCAData()) {
		t.Errorf("expect ca data %v, but got %v", testdata.GetCAData(), newMgr.GetCAData())
	}
	os.RemoveAll(newMgr.GetCaFile())
}

func TestNewCAManagerWithKubeletCertMode(t *testing.T) {
	_, err := NewCAManager(nil, certificate.KubeletCertificateBootstrapMode, "", "", "", []*url.URL{}, []string{})
	if err == nil {
		t.Errorf("expect an error, but got nil")
	}
}

func TestGetCAData(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	client, err := testdata.CreateCertFakeClient("../testdata")
	if err != nil {
		t.Errorf("could not create cert fake client, %v", err)
		return
	}

	mgr, err := NewCAManager(client, certificate.TokenBoostrapMode, "/tmp", "", joinToken, remoteServers, []string{})
	if err != nil {
		t.Errorf("could not new cert manager, %v", err)
	}

	if !bytes.Equal(mgr.GetCAData(), testdata.GetCAData()) {
		t.Errorf("expect ca data %v, but got %v", testdata.GetCAData(), mgr.GetCAData())
	}
	os.RemoveAll(mgr.GetCaFile())
}

func TestGetCaFile(t *testing.T) {
	joinToken := "123456.abcdef1234567890"
	u, _ := url.Parse("http://127.0.0.1")
	remoteServers := []*url.URL{u}
	testcases := map[string]struct {
		workDir string
		path    string
	}{
		"use tmp lib dir": {
			workDir: filepath.Join("/tmp/lib", projectinfo.GetHubName()),
			path:    filepath.Join("/tmp/lib", projectinfo.GetHubName(), "pki", "ca.crt"),
		},
		"define root dir": {
			workDir: "/tmp",
			path:    filepath.Join("/tmp", "pki", "ca.crt"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			client, err := testdata.CreateCertFakeClient("../testdata")
			if err != nil {
				t.Errorf("could not create cert fake client, %v", err)
				return
			}

			mgr, err := NewCAManager(client, certificate.TokenBoostrapMode, tc.workDir, "", joinToken, remoteServers, []string{})
			if err != nil {
				t.Errorf("could not new cert manager, %v", err)
			}

			if mgr.GetCaFile() != tc.path {
				t.Errorf("expect ca.crt file %s, but got %s", tc.path, mgr.GetCaFile())
			}

			os.RemoveAll(mgr.GetCaFile())
		})
	}
}
