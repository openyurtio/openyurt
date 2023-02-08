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

package manager

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	yurtfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func TestFindResponseFilter(t *testing.T) {
	fakeClient := &fake.Clientset{}
	fakeYurtClient := &yurtfake.Clientset{}
	sharedFactory, yurtSharedFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour),
		yurtinformers.NewSharedInformerFactory(fakeYurtClient, 24*time.Hour)
	serializerManager := serializer.NewSerializerManager()
	storageManager, err := disk.NewDiskStorage("/tmp/filter_manager")
	if err != nil {
		t.Fatalf("could not create storage manager, %v", err)
	}
	storageWrapper := cachemanager.NewStorageWrapper(storageManager)
	apiserverAddr := "127.0.0.1:6443"
	stopper := make(chan struct{})
	defer close(stopper)
	sharedFactory.Start(stopper)
	yurtSharedFactory.Start(stopper)

	testcases := map[string]struct {
		enableResourceFilter    bool
		workingMode             string
		disabledResourceFilters []string
		accessServerThroughHub  bool
		enableDummyIf           bool
		userAgent               string
		verb                    string
		path                    string
		mgrIsNil                bool
		isFound                 bool
		names                   []string
	}{
		"disable resource filter": {
			enableResourceFilter: false,
			mgrIsNil:             true,
		},
		"get master service filter": {
			enableResourceFilter:   true,
			accessServerThroughHub: true,
			enableDummyIf:          true,
			userAgent:              "kubelet",
			verb:                   "GET",
			path:                   "/api/v1/services",
			isFound:                true,
			names:                  []string{"masterservice"},
		},
		"get discard cloud service filter": {
			enableResourceFilter:   true,
			accessServerThroughHub: true,
			enableDummyIf:          true,
			userAgent:              "kube-proxy",
			verb:                   "GET",
			path:                   "/api/v1/services",
			isFound:                true,
			names:                  []string{"discardcloudservice"},
		},
		"get service topology filter": {
			enableResourceFilter:   true,
			accessServerThroughHub: true,
			enableDummyIf:          false,
			userAgent:              "kube-proxy",
			verb:                   "GET",
			path:                   "/api/v1/endpoints",
			isFound:                true,
			names:                  []string{"servicetopology"},
		},
		"disable service topology filter": {
			enableResourceFilter:    true,
			accessServerThroughHub:  true,
			disabledResourceFilters: []string{"servicetopology"},
			enableDummyIf:           true,
			userAgent:               "kube-proxy",
			verb:                    "GET",
			path:                    "/api/v1/endpoints",
			isFound:                 false,
		},
		"can't get discard cloud service filter in cloud mode": {
			enableResourceFilter:   true,
			accessServerThroughHub: false,
			workingMode:            "cloud",
			userAgent:              "kube-proxy",
			verb:                   "GET",
			path:                   "/api/v1/services",
			isFound:                false,
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			options := &options.YurtHubOptions{
				EnableResourceFilter:    tt.enableResourceFilter,
				WorkingMode:             tt.workingMode,
				DisabledResourceFilters: make([]string, 0),
				AccessServerThroughHub:  tt.accessServerThroughHub,
				EnableDummyIf:           tt.enableDummyIf,
				NodeName:                "test",
				YurtHubProxySecurePort:  10268,
				HubAgentDummyIfIP:       "127.0.0.1",
				YurtHubProxyHost:        "127.0.0.1",
			}
			options.DisabledResourceFilters = append(options.DisabledResourceFilters, tt.disabledResourceFilters...)

			mgr, _ := NewFilterManager(options, sharedFactory, yurtSharedFactory, serializerManager, storageWrapper, apiserverAddr)
			if tt.mgrIsNil && mgr != nil {
				t.Errorf("expect manager is nil, but got not nil: %v", mgr)
			} else {
				// mgr is nil, complete this test case
				return
			}

			req, err := http.NewRequest(tt.verb, tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			var isFound bool
			var responseFilter filter.ResponseFilter
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				responseFilter, isFound = mgr.FindResponseFilter(req)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isFound != tt.isFound {
				t.Errorf("expect isFound %v, but got %v", tt.isFound, isFound)
				return
			}

			names := strings.Split(responseFilter.Name(), ",")
			if len(tt.names) != len(names) {
				t.Errorf("expect filter names %v, but got %v", tt.names, names)
			}

			for i := range tt.names {
				if tt.names[i] != names[i] {
					t.Errorf("expect filter names %v, but got %v", tt.names, names)
				}
			}
		})
	}
}

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}
