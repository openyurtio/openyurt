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

package filter

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
)

var supportedResourceAndVerbsForFilter = map[string]map[string]sets.String{
	MasterServiceFilterName: {
		"services": sets.NewString("list", "watch"),
	},
	DiscardCloudServiceFilterName: {
		"services": sets.NewString("list", "watch"),
	},
	ServiceTopologyFilterName: {
		"endpoints":      sets.NewString("list", "watch"),
		"endpointslices": sets.NewString("list", "watch"),
	},
}

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
	}{
		"kubelet list services": {
			userAgent:    "kubelet/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services",
			approved:     true,
			resultFilter: []string{MasterServiceFilterName},
		},
		"kubelet watch services": {
			userAgent:    "kubelet/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services?watch=true",
			approved:     true,
			resultFilter: []string{MasterServiceFilterName},
		},
		"kube-proxy list services": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services",
			approved:     true,
			resultFilter: []string{DiscardCloudServiceFilterName},
		},
		"kube-proxy watch services": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/api/v1/services?watch=true",
			approved:     true,
			resultFilter: []string{DiscardCloudServiceFilterName},
		},
		"kube-proxy list endpointslices": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/apis/discovery.k8s.io/v1/endpointslices",
			approved:     true,
			resultFilter: []string{ServiceTopologyFilterName},
		},
		"kube-proxy watch endpointslices": {
			userAgent:    "kube-proxy/v1.20.11",
			verb:         "GET",
			path:         "/apis/discovery.k8s.io/v1/endpointslices?watch=true",
			approved:     true,
			resultFilter: []string{ServiceTopologyFilterName},
		},
		"nginx-ingress-controller list endpoints": {
			userAgent:    "nginx-ingress-controller/v1.1.0",
			verb:         "GET",
			path:         "/api/v1/endpoints",
			approved:     true,
			resultFilter: []string{ServiceTopologyFilterName},
		},
		"nginx-ingress-controller watch endpoints": {
			userAgent:    "nginx-ingress-controller/v1.1.0",
			verb:         "GET",
			path:         "/api/v1/endpoints?watch=true",
			approved:     true,
			resultFilter: []string{ServiceTopologyFilterName},
		},
		"list endpoints without user agent": {
			verb:         "GET",
			path:         "/api/v1/endpoints",
			approved:     false,
			resultFilter: []string{},
		},
		"list configmaps by hub agent": {
			userAgent:    projectinfo.GetHubName(),
			verb:         "GET",
			path:         "/api/v1/configmaps",
			approved:     false,
			resultFilter: []string{},
		},
		"watch configmaps by hub agent": {
			userAgent:    projectinfo.GetHubName(),
			verb:         "GET",
			path:         "/api/v1/configmaps?watch=true",
			approved:     false,
			resultFilter: []string{},
		},
	}

	client := &fake.Clientset{}
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	approver := NewApprover(informerFactory, supportedResourceAndVerbsForFilter)
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

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if approved != tt.approved {
				t.Errorf("expect approved %v, but got %v", tt.approved, approved)
			}

			if len(filterNames) != len(tt.resultFilter) {
				t.Errorf("expect is filter names is %v, but got %v", tt.resultFilter, filterNames)
			}

			for i, name := range filterNames {
				if tt.resultFilter[i] != name {
					t.Errorf("expect is filter names is %v, but got %v", tt.resultFilter, filterNames)
				}
			}
		})
	}
}

func TestAddConfigMap(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := []struct {
		desc                string
		cm                  *v1.ConfigMap
		resultReqKeyToNames map[string]sets.String
	}{
		{
			desc: "add a new filter setting",
			cm: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents":         "nginx-controller",
					"filter_masterservice": "foo, bar",
				},
			},
			resultReqKeyToNames: mergeReqKeyMap(approver.defaultReqKeyToNames, map[string]string{
				"foo/services/list":  "masterservice",
				"foo/services/watch": "masterservice",
				"bar/services/list":  "masterservice",
				"bar/services/watch": "masterservice",
			}),
		},
		{
			desc: "no filter setting exist",
			cm: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents": "nginx-controller",
				},
			},
			resultReqKeyToNames: approver.defaultReqKeyToNames,
		},
	}

	for i, tt := range testcases {
		t.Run(testcases[i].desc, func(t *testing.T) {
			approver.addConfigMap(tt.cm)
			if !reflect.DeepEqual(approver.reqKeyToNames, tt.resultReqKeyToNames) {
				t.Errorf("expect reqkeyToNames is %#+v, but got %#+v", tt.resultReqKeyToNames, approver.reqKeyToNames)
			}
			approver.merge("cleanup", map[string]sets.String{})
		})
	}
}

func TestUpdateConfigMap(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := []struct {
		desc                string
		oldCM               *v1.ConfigMap
		newCM               *v1.ConfigMap
		resultReqKeyToNames map[string]sets.String
	}{
		{
			desc: "add a new filter setting",
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents":           "nginx-controller",
					"filter_servicetopology": "foo, bar",
				},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents":               "nginx-controller",
					"filter_discardcloudservice": "foo, bar",
				},
			},
			resultReqKeyToNames: mergeReqKeyMap(approver.defaultReqKeyToNames, map[string]string{
				"foo/services/list":  "discardcloudservice",
				"foo/services/watch": "discardcloudservice",
				"bar/services/list":  "discardcloudservice",
				"bar/services/watch": "discardcloudservice",
			}),
		},
		{
			desc: "no filter setting changed",
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents":           "nginx-controller",
					"filter_servicetopology": "foo, bar",
				},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurt-hub-cfg",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"cache_agents":           "nginx-controller, agent2",
					"filter_servicetopology": "foo, bar",
				},
			},
			resultReqKeyToNames: approver.defaultReqKeyToNames,
		},
	}

	for i, tt := range testcases {
		t.Run(testcases[i].desc, func(t *testing.T) {
			approver.updateConfigMap(tt.oldCM, tt.newCM)
			if !reflect.DeepEqual(approver.reqKeyToNames, tt.resultReqKeyToNames) {
				t.Errorf("expect reqkeyToName is %#+v, but got %#+v", tt.resultReqKeyToNames, approver.reqKeyToNames)
			}
			approver.merge("cleanup", map[string]sets.String{})
		})
	}
}

func TestMerge(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := map[string]struct {
		action              string
		reqKeyToNamesFromCM map[string]sets.String
		resultReqKeyToNames map[string]sets.String
	}{
		"init req key to name": {
			action:              "init",
			reqKeyToNamesFromCM: map[string]sets.String{},
			resultReqKeyToNames: approver.defaultReqKeyToNames,
		},
		"add some items of req key to name": {
			action: "add",
			reqKeyToNamesFromCM: map[string]sets.String{
				"comp1/resources1/list":  sets.NewString("filter1"),
				"comp2/resources2/watch": sets.NewString("filter2"),
				"comp3/resources3/watch": sets.NewString("filter1"),
			},
			resultReqKeyToNames: mergeReqKeyMap(approver.defaultReqKeyToNames, map[string]string{
				"comp1/resources1/list":  "filter1",
				"comp2/resources2/watch": "filter2",
				"comp3/resources3/watch": "filter1",
			}),
		},
		"update and delete item of req key to name": {
			action: "update",
			reqKeyToNamesFromCM: map[string]sets.String{
				"comp1/resources1/list":  sets.NewString("filter1"),
				"comp2/resources2/watch": sets.NewString("filter3"),
			},
			resultReqKeyToNames: mergeReqKeyMap(approver.defaultReqKeyToNames, map[string]string{
				"comp1/resources1/list":  "filter1",
				"comp2/resources2/watch": "filter3",
			}),
		},
		"update default setting of req key to name": {
			action: "update",
			reqKeyToNamesFromCM: map[string]sets.String{
				"kubelet/services/list":  sets.NewString("filter1"),
				"comp2/resources2/watch": sets.NewString("filter3"),
			},
			resultReqKeyToNames: mergeReqKeyMap(approver.defaultReqKeyToNames, map[string]string{
				"comp2/resources2/watch": "filter3",
				"kubelet/services/list":  "filter1",
			}),
		},
		"clear all user setting of req key to name": {
			action:              "update",
			reqKeyToNamesFromCM: map[string]sets.String{},
			resultReqKeyToNames: approver.defaultReqKeyToNames,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			approver.merge(tt.action, tt.reqKeyToNamesFromCM)
			if !reflect.DeepEqual(approver.reqKeyToNames, tt.resultReqKeyToNames) {
				t.Errorf("expect to get reqKeyToName %#+v, but got %#+v", tt.resultReqKeyToNames, approver.reqKeyToNames)
			}
		})
	}

}

func TestParseRequestSetting(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := map[string]struct {
		filterName    string
		filterSetting string
		resultKeys    []string
	}{
		"old normal filter setting has two components": {
			filterName:    MasterServiceFilterName,
			filterSetting: "foo/services#list;watch,bar/services#list;watch",
			resultKeys:    []string{"foo/services/list", "foo/services/watch", "bar/services/list", "bar/services/watch"},
		},
		"normal filter setting has one component": {
			filterName:    MasterServiceFilterName,
			filterSetting: "foo",
			resultKeys:    []string{"foo/services/list", "foo/services/watch"},
		},
		"normal filter setting has two components": {
			filterName:    MasterServiceFilterName,
			filterSetting: "foo, bar",
			resultKeys:    []string{"foo/services/list", "foo/services/watch", "bar/services/list", "bar/services/watch"},
		},
		"invalid filter name": {
			filterName:    "unknown filter",
			filterSetting: "foo",
			resultKeys:    []string{},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			keys := approver.parseRequestSetting(tt.filterName, tt.filterSetting)

			if !reflect.DeepEqual(keys, tt.resultKeys) {
				t.Errorf("expect request keys %#+v, but got %#+v", tt.resultKeys, keys)
			}
		})
	}
}

func TestHasFilterName(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := map[string]struct {
		key              string
		expectFilterName string
		isFilter         bool
	}{
		"it's not filter": {
			key:              "cache_agents",
			expectFilterName: "",
			isFilter:         false,
		},
		"it's a filter": {
			key:              "filter_masterservice",
			expectFilterName: "masterservice",
			isFilter:         true,
		},
		"only has filter prefix": {
			key:              "filter_",
			expectFilterName: "",
			isFilter:         false,
		},
		"it's a servicetopology filter": {
			key:              "servicetopology",
			expectFilterName: "servicetopology",
			isFilter:         true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			name, ok := approver.hasFilterName(tt.key)
			if name != tt.expectFilterName {
				t.Errorf("expect filter name is %s, but got %s", tt.expectFilterName, name)
			}

			if ok != tt.isFilter {
				t.Errorf("expect has filter bool is %v, but got %v", tt.isFilter, ok)
			}
		})
	}
}

func TestRequestSettingsUpdated(t *testing.T) {
	approver := newApprover(supportedResourceAndVerbsForFilter)
	testcases := map[string]struct {
		old    map[string]string
		new    map[string]string
		result bool
	}{
		"filter setting is not changed": {
			old: map[string]string{
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "",
				"filter_masterservice":       "",
			},
			new: map[string]string{
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "",
				"filter_masterservice":       "",
			},
			result: false,
		},
		"non-filter setting is changed": {
			old: map[string]string{
				"cache_agents":               "foo",
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "",
				"filter_masterservice":       "",
			},
			new: map[string]string{
				"cache_agents":               "bar",
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "",
				"filter_masterservice":       "",
			},
			result: false,
		},
		"filter setting is changed": {
			old: map[string]string{
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "",
				"filter_masterservice":       "",
			},
			new: map[string]string{
				"filter_endpoints":           "coredns/endpoints#list;watch",
				"filter_servicetopology":     "coredns/endpointslices#list;watch",
				"filter_discardcloudservice": "coredns/services#list;watch",
				"filter_masterservice":       "",
			},
			result: true,
		},
		"no prefix filter setting is changed": {
			old: map[string]string{
				"servicetopology":     "coredns",
				"discardcloudservice": "",
				"masterservice":       "",
			},
			new: map[string]string{
				"servicetopology":     "coredns",
				"discardcloudservice": "coredns",
				"masterservice":       "",
			},
			result: true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			needUpdated := approver.requestSettingsUpdated(tt.old, tt.new)
			if needUpdated != tt.result {
				t.Errorf("expect need updated is %v, but got %v", tt.result, needUpdated)
			}
		})
	}
}

func TestGetKeyByRequest(t *testing.T) {
	testcases := map[string]struct {
		userAgent string
		path      string
		resultKey string
	}{
		"list pods by kubelet": {
			userAgent: "kubelet",
			path:      "/api/v1/pods",
			resultKey: "kubelet/pods/list",
		},
		"list nodes by flanneld": {
			userAgent: "flanneld/v1.2",
			path:      "/api/v1/nodes",
			resultKey: "flanneld/nodes/list",
		},
		"list nodes without component": {
			path:      "/api/v1/nodes",
			resultKey: "",
		},
		"list nodes with empty component": {
			userAgent: "",
			path:      "/api/v1/nodes",
			resultKey: "",
		},
	}

	resolver := newTestRequestInfoResolver()
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Errorf("failed to create request, %v", err)
			}
			req.RemoteAddr = "127.0.0.1"

			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			var requestKey string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requestKey = getKeyByRequest(req)
			})

			handler = util.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if requestKey != tt.resultKey {
				t.Errorf("expect req key is %s, but got %s", tt.resultKey, requestKey)
			}
		})
	}
}

func TestReqKey(t *testing.T) {
	testcases := map[string]struct {
		comp     string
		resource string
		verb     string
		result   string
	}{
		"comp is empty": {
			resource: "service",
			verb:     "get",
			result:   "",
		},
		"resource is empty": {
			comp:   "kubelet",
			verb:   "get",
			result: "",
		},
		"verb is empty": {
			comp:     "kubelet",
			resource: "pod",
			result:   "",
		},
		"normal request": {
			comp:     "kubelet",
			resource: "pod",
			verb:     "get",
			result:   "kubelet/pod/get",
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			key := reqKey(tt.comp, tt.resource, tt.verb)
			if key != tt.result {
				t.Errorf("expect req key %s, but got %s", tt.result, key)
			}
		})
	}
}

func mergeReqKeyMap(base map[string]sets.String, m map[string]string) map[string]sets.String {
	reqKeyToNames := make(map[string]sets.String)
	for k, v := range base {
		reqKeyToNames[k] = sets.NewString(v.UnsortedList()...)
	}

	for k, v := range m {
		if _, ok := reqKeyToNames[k]; ok {
			reqKeyToNames[k].Insert(v)
		} else {
			reqKeyToNames[k] = sets.NewString(v)
		}
	}

	return reqKeyToNames
}

func newApprover(filterSupportedResAndVerbs map[string]map[string]sets.String) *approver {
	na := &approver{
		reqKeyToNames:                      make(map[string]sets.String),
		supportedResourceAndVerbsForFilter: filterSupportedResAndVerbs,
		stopCh:                             make(chan struct{}),
	}

	defaultReqKeyToFilterNames := make(map[string]sets.String)
	for name, setting := range SupportedComponentsForFilter {
		for _, key := range na.parseRequestSetting(name, setting) {
			if _, ok := defaultReqKeyToFilterNames[key]; !ok {
				defaultReqKeyToFilterNames[key] = sets.NewString()
			}
			defaultReqKeyToFilterNames[key].Insert(name)
		}
	}
	na.defaultReqKeyToNames = defaultReqKeyToFilterNames

	na.merge("init", na.defaultReqKeyToNames)
	return na
}
