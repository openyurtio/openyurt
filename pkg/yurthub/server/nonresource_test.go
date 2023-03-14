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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

var rootDir = "/tmp/cache-local"

var (
	notFoundError = errors.New("not found")
	internalError = errors.New("internal error")
)

func TestLocalCacheHandler(t *testing.T) {
	dStorage, err := disk.NewDiskStorage(rootDir)
	if err != nil {
		t.Errorf("disk initialize error: %v", err)
	}

	sw := cachemanager.NewStorageWrapper(dStorage)
	//u, _ := url.Parse("https://10.10.10.113:6443")
	fakeHealthChecker := healthchecker.NewFakeChecker(false, nil)

	rcm, err := rest.NewRestConfigManager(nil, fakeHealthChecker)
	if err != nil {
		t.Fatal(err)
	}

	testcases := map[string]struct {
		path             string
		initData         []byte
		statusCode       int
		metav1StatusCode int
	}{
		"failed to get from local cache, because it does not exist": {
			path:             "/version",
			initData:         nil,
			statusCode:       http.StatusNotFound,
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
			key := storage.ClusterInfoKey{
				ClusterInfoType: nonResourceReqPaths[tt.path],
				UrlPath:         tt.path,
			}
			if tt.initData != nil {
				sw.SaveClusterInfo(key, tt.initData)
			}

			req, err := http.NewRequest("GET", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			localCacheHandler(nonResourceHandler, rcm, sw, tt.path).ServeHTTP(resp, req)

			if resp.Code != tt.statusCode {
				t.Errorf("expect status code %d, but got %d", tt.statusCode, resp.Code)
			}

			b, _ := io.ReadAll(resp.Body)
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
					t.Errorf("expect response data %s, but got %s", tt.initData, b)
				}
			}

			if err := os.RemoveAll(filepath.Join(rootDir, tt.path)); err != nil {
				t.Errorf("failed to remove file, %v", err)
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
		"failed to get non resource because of internal error": {
			path:             "/apis/discovery.k8s.io/v1beta1",
			err:              internalError,
			statusCode:       http.StatusInternalServerError,
			metav1StatusCode: http.StatusInternalServerError,
		},
		"failed to get non resource because it does not exist": {
			path:             "/apis/discovery.k8s.io/v1",
			err:              notFoundError,
			statusCode:       http.StatusNotFound,
			metav1StatusCode: http.StatusNotFound,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			fakeClient := &fakerest.RESTClient{
				Client: fakerest.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
					switch tt.err {
					case nil:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader(string(tt.initData))),
						}, nil
					case notFoundError:
						return &http.Response{
							StatusCode: http.StatusNotFound,
							Body:       io.NopCloser(bytes.NewReader([]byte{})),
						}, nil
					default:
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       io.NopCloser(bytes.NewReader([]byte{})),
						}, err
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

			b, _ := io.ReadAll(resp.Body)
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
		})
	}
}
