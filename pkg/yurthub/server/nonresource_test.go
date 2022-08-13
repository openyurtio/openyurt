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

package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

var rootDir = "/tmp/cache-local"

func TestNonResourceInfoCache(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("disk initialize error: %v", err)
	}

	sw := cachemanager.NewStorageWrapper(dStorage)

	testcases := map[string]struct {
		Accept      string
		Verb        string
		Path        string
		Code        int
		ContentType string
		Response    string
	}{
		"version info": {
			Accept:      "application/json",
			Verb:        "GET",
			Path:        "/version",
			Code:        http.StatusOK,
			ContentType: "application/json",
			Response:    "fake-non-resource-info-/version",
		},

		"discovery v1": {
			Accept:      "application/json",
			Verb:        "GET",
			Path:        "/apis/discovery.k8s.io/v1",
			Code:        http.StatusOK,
			ContentType: "application/json",
			Response:    "fake-non-resource-info-/apis/discovery.k8s.io/v1",
		},
		"discovery v1beta1": {
			Accept:      "application/json",
			Verb:        "GET",
			Path:        "/apis/discovery.k8s.io/v1beta1",
			Code:        http.StatusOK,
			ContentType: "application/json",
			Response:    "fake-non-resource-info-/apis/discovery.k8s.io/v1beta1",
		},
	}

	for k, tt := range testcases {
		//test for caching the response
		t.Run(k, func(t *testing.T) {
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				cacheNonResourceInfo(w, req, sw, fmt.Sprintf("non-reosurce-info%s", req.URL.Path), req.URL.Path, true)
			})
			req, _ := http.NewRequest(tt.Verb, tt.Path, nil)
			if len(tt.Accept) != 0 {
				req.Header.Set("Accept", tt.Accept)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.Code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.Code)
			}
			if string(resp.Body.Bytes()) != tt.Response {
				t.Errorf("got response %s, but expect %s", string(resp.Body.Bytes()), tt.Response)
			}

		})

		//test for query the cache
		t.Run(k, func(t *testing.T) {
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				cacheNonResourceInfo(w, req, sw, fmt.Sprintf("non-reosurce-info%s", req.URL.Path), req.URL.Path, false)
			})
			req, _ := http.NewRequest(tt.Verb, tt.Path, nil)
			if len(tt.Accept) != 0 {
				req.Header.Set("Accept", tt.Accept)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			result := resp.Result()
			if result.StatusCode != tt.Code {
				t.Errorf("got status code %d, but expect %d", result.StatusCode, tt.Code)
			}
			if string(resp.Body.Bytes()) != tt.Response {
				t.Errorf("got response %s, but expect %s", string(resp.Body.Bytes()), tt.Response)
			}

		})
	}

	os.RemoveAll(rootDir)
}
