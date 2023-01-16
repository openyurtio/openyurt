/*
Copyright 2022 The OpenYurt Authors.

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
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/token/testdata"
)

var (
	testDir = "/tmp/server/cert"
)

func TestUpdateTokenHandler(t *testing.T) {
	u, _ := url.Parse("https://10.10.10.113:6443")
	remoteServers := []*url.URL{u}
	certIPs := []net.IP{net.ParseIP("127.0.0.1")}
	client, err := testdata.CreateCertFakeClient("../certificate/token/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	certManager, err := token.NewYurtHubCertManager(&token.CertificateManagerConfiguration{
		NodeName:      "foo",
		RemoteServers: remoteServers,
		CertIPs:       certIPs,
		RootDir:       testDir,
		JoinToken:     "123456.abcdef1234567890",
		Client:        client,
	})
	if err != nil {
		t.Errorf("failed to create certManager, %v", err)
		return
	}
	certManager.Start()
	defer certManager.Stop()
	defer os.RemoveAll(testDir)

	err = wait.PollImmediate(2*time.Second, 1*time.Minute, func() (done bool, err error) {
		if certManager.Ready() {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("certificates are not ready, %v", err)
	}

	testcases := map[string]struct {
		body       map[string]string
		statusCode int
	}{
		"failed to update join token": {
			body:       map[string]string{},
			statusCode: http.StatusBadRequest,
		},
		"update join token normally": {
			body: map[string]string{
				tokenKey: "123456.abcdef1234567890",
			},
			statusCode: http.StatusOK,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req, err := http.NewRequest("POST", "", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			updateTokenHandler(certManager).ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Errorf("expect status code %d, but got %d", tt.statusCode, resp.Code)
			}
		})
	}
}
