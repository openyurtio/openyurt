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

package approver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/forwardkubesvctraffic"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	util2 "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestApprove(t *testing.T) {
	testcases := map[string]struct {
		userAgent    string
		verb         string
		path         string
		approved     bool
		resultFilter []string
		workingMode  util2.WorkingMode
	}{
		"kubelet list services": {
			userAgent:    "kubelet/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services",
			approved:     true,
			resultFilter: []string{masterservice.FilterName},
			workingMode:  util2.WorkingModeCloud,
		},
		"kubelet watch services": {
			userAgent:    "kubelet/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services?watch=true",
			approved:     true,
			resultFilter: []string{masterservice.FilterName},
			workingMode:  util2.WorkingModeCloud,
		},
		"kube-proxy list services": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services",
			approved:     true,
			resultFilter: []string{discardcloudservice.FilterName, nodeportisolation.FilterName},
			workingMode:  util2.WorkingModeCloud,
		},
		"kube-proxy watch services": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services?watch=true",
			approved:     true,
			resultFilter: []string{discardcloudservice.FilterName, nodeportisolation.FilterName},
			workingMode:  util2.WorkingModeEdge,
		},
		"kube-proxy list endpointslices": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/apis/discovery.k8s.io/v1/endpointslices",
			approved:     true,
			resultFilter: []string{servicetopology.FilterName, forwardkubesvctraffic.FilterName},
			workingMode:  util2.WorkingModeEdge,
		},
		"kube-proxy watch endpointslices": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/apis/discovery.k8s.io/v1/endpointslices?watch=true",
			approved:     true,
			resultFilter: []string{servicetopology.FilterName, forwardkubesvctraffic.FilterName},
			workingMode:  util2.WorkingModeEdge,
		},
		"nginx-ingress-controller list endpoints": {
			userAgent:    "nginx-ingress-controller/v1.1.0",
			verb:         "GET",
			path:         "/api/v1/endpoints",
			approved:     true,
			resultFilter: []string{servicetopology.FilterName},
			workingMode:  util2.WorkingModeEdge,
		},
		"nginx-ingress-controller watch endpoints": {
			userAgent:    "nginx-ingress-controller/v1.1.0",
			verb:         "GET",
			path:         "/api/v1/endpoints?watch=true",
			approved:     true,
			resultFilter: []string{servicetopology.FilterName},
			workingMode:  util2.WorkingModeEdge,
		},
		"list endpoints without user agent": {
			verb:         "GET",
			path:         "/api/v1/endpoints",
			approved:     false,
			resultFilter: []string{},
			workingMode:  util2.WorkingModeEdge,
		},
		"list configmaps by hub agent": {
			userAgent:    projectinfo.GetHubName(),
			verb:         "GET",
			path:         "/api/v1/configmaps",
			approved:     false,
			resultFilter: []string{},
			workingMode:  util2.WorkingModeEdge,
		},
		"watch configmaps by hub agent": {
			userAgent:    projectinfo.GetHubName(),
			verb:         "GET",
			path:         "/api/v1/configmaps?watch=true",
			approved:     false,
			resultFilter: []string{},
			workingMode:  util2.WorkingModeCloud,
		},
		"watch configmaps by unknown agent": {
			userAgent:    "unknown-agent",
			verb:         "GET",
			path:         "/api/v1/configmaps?watch=true",
			approved:     false,
			resultFilter: []string{},
			workingMode:  util2.WorkingModeCloud,
		},
	}

	nodeName := "foo"
	client := &fake.Clientset{}
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	manager := configuration.NewConfigurationManager(nodeName, informerFactory)
	approver := NewApprover(nodeName, manager)
	stopper := make(chan struct{})
	defer close(stopper)
	informerFactory.Start(stopper)
	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, err := http.NewRequest(tt.verb, tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			var approved bool
			var filterNames []string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				approved, filterNames = approver.Approve(req)
			})

			handler = util.WithRequestClientComponent(handler, tt.workingMode)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if approved != tt.approved {
				t.Errorf("expect approved %v, but got %v", tt.approved, approved)
			}

			if len(filterNames) != len(tt.resultFilter) {
				t.Errorf("expect is filter names is %v, but got %v", tt.resultFilter, filterNames)
				return
			}

			if !sets.New[string](filterNames...).Equal(sets.New[string](tt.resultFilter...)) {
				t.Errorf("expect is filter names is %v, but got %v", tt.resultFilter, filterNames)
			}
		})
	}
}
