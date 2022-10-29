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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

var rootDir = "/tmp/cache-local"

func TestLocalCacheHandler(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("disk initialize error: %v", err)
	}

	sw := cachemanager.NewStorageWrapper(dStorage)
	u, _ := url.Parse("https://10.10.10.113:6443")
	fakeHealthChecker := healthchecker.NewFakeChecker(false, nil)
	cfg := &config.YurtHubConfiguration{
		RemoteServers: []*url.URL{u},
	}

	rcm, err := rest.NewRestConfigManager(cfg, nil, fakeHealthChecker)
	if err != nil {
		t.Fatal(err)
	}

	testcases := map[string]struct {
		path             string
		initData         []byte
		statusCode       int
		metav1StatusCode int
	}{
		"failed to get from local cache": {
			path:             "/version",
			initData:         []byte{},
			statusCode:       http.StatusOK,
			metav1StatusCode: http.StatusInternalServerError,
		},
		"get from local cache normally": {
			path:       "/version",
			initData:   []byte("v1.0.0"),
			statusCode: http.StatusOK,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			key := fmt.Sprintf("non-reosurce-info%s", tt.path)
			sw.UpdateRaw(key, tt.initData)

			req, err := http.NewRequest("GET", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			localCacheHandler(nonResourceHandler, rcm, sw, tt.path).ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Errorf("expect status code %d, but got %d", tt.statusCode, resp.Code)
			}

			b, _ := ioutil.ReadAll(resp.Body)
			if tt.metav1StatusCode != 0 {
				var status metav1.Status
				err := json.Unmarshal(b, &status)
				if err != nil {
					t.Errorf("failed to unmarshal response, %v", err)
				}

				if int(status.Code) != tt.metav1StatusCode {
					t.Errorf("expect metav1 status code %d, but got %d", tt.metav1StatusCode, status.Code)
				}

			} else {
				if !bytes.Equal(b, tt.initData) {
					t.Errorf("expect response data %v, but got %v", tt.initData, b)
				}

			}

			if err := sw.Delete(key); err != nil {
				t.Errorf("failed to remove %s, %v", key, err)
			}
		})
	}
}

func TestNonResourceHandler(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("disk initialize error: %v", err)
	}

	sw := cachemanager.NewStorageWrapper(dStorage)

	testcases := map[string]struct {
		path             string
		initData         []byte
		err              error
		statusCode       int
		metav1StatusCode int
	}{
		"get non resource normally": {
			path:       "/apis/discovery.k8s.io/v1",
			initData:   []byte("fake resource"),
			statusCode: http.StatusOK,
		},
		"failed to get non resource": {
			path:             "/apis/discovery.k8s.io/v1beta1",
			err:              errors.New("fake error"),
			statusCode:       http.StatusOK,
			metav1StatusCode: http.StatusInternalServerError,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			fakeClient := &fakerest.RESTClient{
				Client: fakerest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
					if tt.err == nil {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       ioutil.NopCloser(strings.NewReader(string(tt.initData))),
						}, nil
					} else {
						return nil, tt.err
					}
				}),
				NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
				GroupVersion:         schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"},
				VersionedAPIPath:     tt.path,
			}
			var kubeClient kubernetes.Clientset
			kubeClient.DiscoveryClient = discovery.NewDiscoveryClient(fakeClient)

			req, err := http.NewRequest("GET", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			nonResourceHandler(&kubeClient, sw, tt.path).ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Errorf("expect status code %d, but got %d", tt.statusCode, resp.Code)
			}

			b, _ := ioutil.ReadAll(resp.Body)
			if tt.metav1StatusCode != 0 {
				var status metav1.Status
				err := json.Unmarshal(b, &status)
				if err != nil {
					t.Errorf("failed to unmarshal response, %v", err)
				}

				if int(status.Code) != tt.metav1StatusCode {
					t.Errorf("expect metav1 status code %d, but got %d", tt.metav1StatusCode, status.Code)
				}

			} else {
				if !bytes.Equal(b, tt.initData) {
					t.Errorf("expect response data %v, but got %v", tt.initData, b)
				}
			}

			key := fmt.Sprintf("non-reosurce-info%s", tt.path)
			if err := sw.Delete(key); err != nil {
				t.Errorf("failed to remove %s, %v", key, err)
			}
		})
	}
}
