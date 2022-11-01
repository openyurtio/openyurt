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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/hubself"
)

func TestUpdateTokenHandler(t *testing.T) {
	certMgr := &hubself.FakeYurtHubCertManager{}

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
				tokenKey: "123456",
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
			updateTokenHandler(certMgr).ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Errorf("expect status code %d, but got %d", tt.statusCode, resp.Code)
			}
		})
	}
}
