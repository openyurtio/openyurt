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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
)

func TestFindResponseFilter(t *testing.T) {
	fakeClient := &fake.Clientset{}
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		enableResourceFilter    bool
		workingMode             string
		disabledResourceFilters []string
		enableDummyIf           bool
		userAgent               string
		verb                    string
		path                    string
		isFound                 bool
		names                   sets.Set[string]
	}{
		"disable resource filter": {
			enableResourceFilter: false,
			enableDummyIf:        true,
			userAgent:            "kubelet",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              false,
			names:                sets.New[string](),
		},
		"get master service filter": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "kubelet",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("masterservice"),
		},
		"get discard cloud service and node port isolation filter": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("discardcloudservice", "nodeportisolation"),
		},
		"get service topology filter": {
			enableResourceFilter: true,
			enableDummyIf:        false,
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/endpoints",
			isFound:              true,
			names:                sets.New("servicetopology"),
		},
		"disable service topology filter": {
			enableResourceFilter:    true,
			disabledResourceFilters: []string{"servicetopology"},
			enableDummyIf:           true,
			userAgent:               "kube-proxy",
			verb:                    "GET",
			path:                    "/api/v1/endpoints",
			isFound:                 false,
		},
		"can't get discard cloud service filter in cloud mode": {
			enableResourceFilter: true,
			workingMode:          "cloud",
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("nodeportisolation"),
		},
		"reject by approver for unknown component": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "unknown-agent",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              false,
			names:                sets.New[string](),
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			options := &options.YurtHubOptions{
				EnableResourceFilter:    tt.enableResourceFilter,
				WorkingMode:             tt.workingMode,
				DisabledResourceFilters: make([]string, 0),
				EnableDummyIf:           tt.enableDummyIf,
				NodeName:                "test",
				YurtHubProxySecurePort:  10268,
				HubAgentDummyIfIP:       "127.0.0.1",
				YurtHubProxyHost:        "127.0.0.1",
			}
			options.DisabledResourceFilters = append(options.DisabledResourceFilters, tt.disabledResourceFilters...)

			sharedFactory, nodePoolFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour),
				dynamicinformer.NewDynamicSharedInformerFactory(fakeDynamicClient, 24*time.Hour)

			configManager := configuration.NewConfigurationManager(options.NodeName, sharedFactory)
			stopper := make(chan struct{})
			defer close(stopper)

			finder, _ := NewFilterManager(options, sharedFactory, nodePoolFactory, fakeClient, serializerManager, configManager)

			sharedFactory.Start(stopper)
			nodePoolFactory.Start(stopper)

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
				responseFilter, isFound = finder.FindResponseFilter(req)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isFound != tt.isFound {
				t.Errorf("expect found result %v, but got %v", tt.isFound, isFound)
			} else if !tt.isFound {
				// skip checking filter names because no filter is found.
				return
			}

			names := strings.Split(responseFilter.Name(), ",")
			if !tt.names.Equal(sets.New(names...)) {
				t.Errorf("expect filter names %v, but got %v", sets.List(tt.names), names)
			}
		})
	}
}

func TestFindObjectFilter(t *testing.T) {
	fakeClient := &fake.Clientset{}
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		enableResourceFilter    bool
		workingMode             string
		disabledResourceFilters []string
		enableDummyIf           bool
		userAgent               string
		verb                    string
		path                    string
		isFound                 bool
		names                   sets.Set[string]
	}{
		"disable resource filter": {
			enableResourceFilter: false,
			enableDummyIf:        true,
			userAgent:            "kubelet",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              false,
			names:                sets.New[string](),
		},
		"get master service filter": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "kubelet",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("masterservice"),
		},
		"get discard cloud service and node port isolation filter": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("discardcloudservice", "nodeportisolation"),
		},
		"get service topology filter": {
			enableResourceFilter: true,
			enableDummyIf:        false,
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/endpoints",
			isFound:              true,
			names:                sets.New("servicetopology"),
		},
		"disable service topology filter": {
			enableResourceFilter:    true,
			disabledResourceFilters: []string{"servicetopology"},
			enableDummyIf:           true,
			userAgent:               "kube-proxy",
			verb:                    "GET",
			path:                    "/api/v1/endpoints",
			isFound:                 false,
		},
		"can't get discard cloud service filter in cloud mode": {
			enableResourceFilter: true,
			workingMode:          "cloud",
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              true,
			names:                sets.New("nodeportisolation"),
		},
		"reject by approver for unknown component": {
			enableResourceFilter: true,
			enableDummyIf:        true,
			userAgent:            "unknown-agent",
			verb:                 "GET",
			path:                 "/api/v1/services",
			isFound:              false,
			names:                sets.New[string](),
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			options := &options.YurtHubOptions{
				EnableResourceFilter:    tt.enableResourceFilter,
				WorkingMode:             tt.workingMode,
				DisabledResourceFilters: make([]string, 0),
				EnableDummyIf:           tt.enableDummyIf,
				NodeName:                "test",
				YurtHubProxySecurePort:  10268,
				HubAgentDummyIfIP:       "127.0.0.1",
				YurtHubProxyHost:        "127.0.0.1",
			}
			options.DisabledResourceFilters = append(options.DisabledResourceFilters, tt.disabledResourceFilters...)

			sharedFactory, nodePoolFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour),
				dynamicinformer.NewDynamicSharedInformerFactory(fakeDynamicClient, 24*time.Hour)

			configManager := configuration.NewConfigurationManager(options.NodeName, sharedFactory)
			stopper := make(chan struct{})
			defer close(stopper)

			finder, _ := NewFilterManager(options, sharedFactory, nodePoolFactory, fakeClient, serializerManager, configManager)

			sharedFactory.Start(stopper)
			nodePoolFactory.Start(stopper)

			req, err := http.NewRequest(tt.verb, tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			var isFound bool
			var objectFilter filter.ObjectFilter
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				objectFilter, isFound = finder.FindObjectFilter(req)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isFound != tt.isFound {
				t.Errorf("expect found result %v, but got %v", tt.isFound, isFound)
			} else if !tt.isFound {
				// skip checking filter names because no filter is found.
				return
			}

			names := strings.Split(objectFilter.Name(), ",")
			if !tt.names.Equal(sets.New(names...)) {
				t.Errorf("expect filter names %v, but got %v", sets.List(tt.names), names)
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

func TestHasSynced(t *testing.T) {
	fakeClient := &fake.Clientset{}
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	serializerManager := serializer.NewSerializerManager()

	testcases := map[string]struct {
		enableResourceFilter    bool
		workingMode             string
		disabledResourceFilters []string
		enableDummyIf           bool
		userAgent               string
		verb                    string
		path                    string
		hasSynced               bool
	}{
		"has synced by disabling resource filter": {
			enableResourceFilter: false,
			enableDummyIf:        true,
			userAgent:            "kubelet",
			verb:                 "GET",
			path:                 "/api/v1/services",
			hasSynced:            true,
		},
		"has synced by disabling service topology filter": {
			enableResourceFilter:    true,
			disabledResourceFilters: []string{"servicetopology"},
			enableDummyIf:           true,
			userAgent:               "kube-proxy",
			verb:                    "GET",
			path:                    "/api/v1/endpoints",
			hasSynced:               true,
		},
		"not synced by setting service topology filter": {
			enableResourceFilter: true,
			enableDummyIf:        false,
			userAgent:            "kube-proxy",
			verb:                 "GET",
			path:                 "/api/v1/endpoints",
			hasSynced:            false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			options := &options.YurtHubOptions{
				EnableResourceFilter:    tc.enableResourceFilter,
				WorkingMode:             tc.workingMode,
				DisabledResourceFilters: make([]string, 0),
				EnableDummyIf:           tc.enableDummyIf,
				NodeName:                "test",
				YurtHubProxySecurePort:  10268,
				HubAgentDummyIfIP:       "127.0.0.1",
				YurtHubProxyHost:        "127.0.0.1",
			}
			options.DisabledResourceFilters = append(options.DisabledResourceFilters, tc.disabledResourceFilters...)
			sharedFactory, nodePoolFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour),
				dynamicinformer.NewDynamicSharedInformerFactory(fakeDynamicClient, 24*time.Hour)
			configManager := configuration.NewConfigurationManager(options.NodeName, sharedFactory)

			finder, _ := NewFilterManager(options, sharedFactory, nodePoolFactory, fakeClient, serializerManager, configManager)
			hasSynced := finder.HasSynced()
			if hasSynced != tc.hasSynced {
				t.Errorf("expect synced result: %v, but got %v", tc.hasSynced, hasSynced)
			}
		})
	}

}
