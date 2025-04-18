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

package nonresourcerequest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/testdata"
	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
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
	servers := map[*url.URL]bool{
		{Host: "10.10.10.113:6443"}: false,
	}
	fakeHealthChecker := fakeHealthChecker.NewFakeChecker(servers)

	u, _ := url.Parse("https://10.10.10.113:6443")
	remoteServers := []*url.URL{u}

	testDir, err := os.MkdirTemp("", "test-client")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	nodeName := "foo"
	client, err := testdata.CreateCertFakeClient("../../certificate/testdata")
	if err != nil {
		t.Errorf("failed to create cert fake client, %v", err)
		return
	}
	certManager, err := manager.NewYurtHubCertManager(&options.YurtHubOptions{
		NodeName:      nodeName,
		RootDir:       testDir,
		YurtHubHost:   "127.0.0.1",
		JoinToken:     "123456.abcdef1234567890",
		ClientForTest: client,
	}, remoteServers)
	if err != nil {
		t.Errorf("failed to create certManager, %v", err)
		return
	}
	certManager.Start()
	defer certManager.Stop()
	defer os.RemoveAll(testDir)

	err = wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 1*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		if certManager.Ready() {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Errorf("certificates are not ready, %v", err)
	}

	transportManager, err := transport.NewTransportAndClientManager(remoteServers, 10, certManager, context.Background().Done())
	if err != nil {
		t.Fatalf("could not new transport manager, %v", err)
	}

	testcases := map[string]struct {
		path             string
		initData         []byte
		statusCode       int
		metav1StatusCode int
	}{
		"can not get from local cache, because it does not exist": {
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
			key := &storage.ClusterInfoKey{
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
			localCacheHandler(nonResourceHandler, fakeHealthChecker, transportManager, sw, tt.path).ServeHTTP(resp, req)

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
		"can not get non resource because of internal error": {
			path:             "/apis/discovery.k8s.io/v1beta1",
			err:              internalError,
			statusCode:       http.StatusInternalServerError,
			metav1StatusCode: http.StatusInternalServerError,
		},
		"can not get non resource because it does not exist": {
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
