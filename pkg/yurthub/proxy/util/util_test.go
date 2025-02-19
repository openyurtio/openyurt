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

package util

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var serviceGVR = &schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "services",
}

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}

func TestWithIsRequestForPoolScopeMetadata(t *testing.T) {
	testcases := map[string]struct {
		userAgent                     string
		verb                          string
		path                          string
		isRequestForPoolScopeMetadata bool
	}{
		"list service resource": {
			userAgent:                     "kubelet",
			verb:                          "GET",
			path:                          "/api/v1/services",
			isRequestForPoolScopeMetadata: true,
		},

		"get node resource": {
			userAgent:                     "flanneld/0.11.0",
			verb:                          "GET",
			path:                          "/api/v1/nodes/mynode",
			isRequestForPoolScopeMetadata: false,
		},
	}

	resolver := newTestRequestInfoResolver()

	clientset := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(clientset, 0)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.verb, tc.path, nil)
			if len(tc.userAgent) != 0 {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			storageMap := map[string]kstorage.Interface{
				serviceGVR.String(): nil,
			}
			dsm := storage.NewDummyStorageManager(storageMap)

			tmpDir, err := os.MkdirTemp("", "test")
			if err != nil {
				t.Fatalf("failed to make temp dir, %v", err)
			}
			restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

			poolScopeResources := []schema.GroupVersionResource{
				{Group: "", Version: "v1", Resource: "services"},
				{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"},
			}

			healthChecher := fakeHealthChecker.NewFakeChecker(map[*url.URL]bool{})
			loadBalancer := remote.NewLoadBalancer("round-robin", []*url.URL{}, nil, nil, healthChecher, nil, context.Background().Done())
			cfg := &config.YurtHubConfiguration{
				PoolScopeResources:       poolScopeResources,
				RESTMapperManager:        restMapperManager,
				SharedFactory:            factory,
				LoadBalancerForLeaderHub: loadBalancer,
			}
			rmm := multiplexer.NewRequestMultiplexerManager(cfg, dsm, healthChecher)

			var isRequestForPoolScopeMetadata bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				isRequestForPoolScopeMetadata, _ = util.IsRequestForPoolScopeMetadataFrom(ctx)
			})

			handler = WithIsRequestForPoolScopeMetadata(handler, rmm.IsRequestForPoolScopeMetadata)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isRequestForPoolScopeMetadata != tc.isRequestForPoolScopeMetadata {
				t.Errorf("%s: expect isRequestForPoolScopeMetadata %v, but got %v", k, tc.isRequestForPoolScopeMetadata, isRequestForPoolScopeMetadata)
			}
		})
	}
}

func TestWithPartialObjectMetadataRequest(t *testing.T) {
	testcases := map[string]struct {
		Verb         string
		Path         string
		Header       map[string]string
		IsPartialReq bool
		ConvertGVK   schema.GroupVersionKind
	}{
		"kubelet request": {
			Verb: "GET",
			Path: "/api/v1/nodes/mynode",
			Header: map[string]string{
				"User-Agent": "kubelet",
			},
			IsPartialReq: false,
		},
		"flanneld list request by partial object metadata request": {
			Verb: "GET",
			Path: "/api/v1/nodes",
			Header: map[string]string{
				"User-Agent": "flanneld/0.11.0",
				"Accept":     "application/vnd.kubernetes.protobuf;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1,application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1,application/json",
			},
			IsPartialReq: true,
			ConvertGVK: schema.GroupVersionKind{
				Group:   "meta.k8s.io",
				Version: "v1",
				Kind:    "PartialObjectMetadataList",
			},
		},
		"flanneld get request by partial object metadata request": {
			Verb: "GET",
			Path: "/api/v1/nodes/mynode",
			Header: map[string]string{
				"User-Agent": "flanneld/0.11.0",
				"Accept":     "application/vnd.kubernetes.protobuf;as=PartialObjectMetadata;g=meta.k8s.io;v=v1,application/json;as=PartialObjectMetadata;g=meta.k8s.io;v=v1,application/json",
			},
			IsPartialReq: true,
			ConvertGVK: schema.GroupVersionKind{
				Group:   "meta.k8s.io",
				Version: "v1",
				Kind:    "PartialObjectMetadata",
			},
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			for k, v := range tc.Header {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = "127.0.0.1"

			var isPartialReq bool
			var convertGVK *schema.GroupVersionKind
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				convertGVK, isPartialReq = util.ConvertGVKFrom(ctx)
			})

			handler = WithRequestClientComponent(handler)
			handler = WithPartialObjectMetadataRequest(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isPartialReq != tc.IsPartialReq {
				t.Errorf("expect isPartialReq %v, but got %v", tc.IsPartialReq, isPartialReq)
			}

			if tc.IsPartialReq {
				if !reflect.DeepEqual(tc.ConvertGVK, *convertGVK) {
					t.Errorf("expect convert gvk %v, but got %v", tc.ConvertGVK, *convertGVK)
				}
			}
		})
	}
}

func TestWithRequestContentType(t *testing.T) {
	testcases := map[string]struct {
		Accept      string
		Verb        string
		Path        string
		Code        int
		ContentType string
	}{
		"resource request": {
			Accept:      "application/json",
			Verb:        "GET",
			Path:        "/api/v1/nodes/mynode",
			Code:        http.StatusOK,
			ContentType: "application/json",
		},

		"not resource request": {
			Accept:      "application/vnd.kubernetes.protobuf",
			Verb:        "GET",
			Path:        "/healthz",
			Code:        http.StatusOK,
			ContentType: "",
		},
		"no accept type": {
			Verb:        "POST",
			Path:        "/api/v1/nodes/mynode",
			Code:        http.StatusOK,
			ContentType: "",
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
		if len(tc.Accept) != 0 {
			req.Header.Set("Accept", tc.Accept)
		}
		req.RemoteAddr = "127.0.0.1"

		var contentType string
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			contentType, _ = util.ReqContentTypeFrom(ctx)
			w.WriteHeader(http.StatusOK)
		})

		handler = WithRequestContentType(handler)
		handler = filters.WithRequestInfo(handler, resolver)

		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		result := resp.Result()
		if result.StatusCode != tc.Code {
			t.Errorf("%s: expect status code: %d, but got %d", k, tc.Code, result.StatusCode)
		}

		if result.StatusCode == http.StatusOK {
			if contentType != tc.ContentType {
				t.Errorf("%s: expect content type %s, but got %s", k, tc.ContentType, contentType)
			}
		}
	}
}

func TestWithRequestClientComponent(t *testing.T) {
	testcases := map[string]struct {
		UserAgent          string
		Verb               string
		Path               string
		ClientComponent    string
		TruncatedComponent string
	}{
		"kubelet request": {
			UserAgent:          "kubelet123",
			Verb:               "GET",
			Path:               "/api/v1/nodes/mynode",
			ClientComponent:    "kubelet123",
			TruncatedComponent: "kubelet123",
		},

		"flanneld request": {
			UserAgent:          "flanneld/0.11.0",
			Verb:               "GET",
			Path:               "/api/v1/nodes/mynode",
			ClientComponent:    "flanneld/0.11.0",
			TruncatedComponent: "flanneld",
		},
		"not resource request": {
			UserAgent:       "kubelet",
			Verb:            "POST",
			Path:            "/healthz",
			ClientComponent: "",
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			if len(tc.UserAgent) != 0 {
				req.Header.Set("User-Agent", tc.UserAgent)
			}
			req.RemoteAddr = "127.0.0.1"

			var clientComponent, truncatedComponent string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				clientComponent, _ = util.ClientComponentFrom(ctx)
				truncatedComponent, _ = util.TruncatedClientComponentFrom(ctx)
			})

			handler = WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if clientComponent != tc.ClientComponent {
				t.Errorf("expect client component %s, but got %s", tc.ClientComponent, clientComponent)
			}

			if truncatedComponent != tc.TruncatedComponent {
				t.Errorf("expect truncated component %s, but got %s", tc.TruncatedComponent, truncatedComponent)
			}
		})
	}
}

func TestWithRequestTimeout(t *testing.T) {
	testcases := map[string]struct {
		Verb    string
		Path    string
		Timeout int
		Err     error
	}{
		"no timeout": {
			Verb:    "GET",
			Path:    "/api/v1/pods?resourceVersion=1494416105&timeout=5s&timeoutSeconds=5&watch=true",
			Timeout: 19,
			Err:     nil,
		},

		"timeout cancel": {
			Verb:    "GET",
			Path:    "/api/v1/pods?resourceVersion=1494416105&timeout=5s&timeoutSeconds=5&watch=true",
			Timeout: 21,
			Err:     context.DeadlineExceeded,
		},

		"no reduce timeout cancel list": {
			Verb:    "GET",
			Path:    "/api/v1/pods?resourceVersion=1494416105&timeout=5s&timeoutSeconds=5",
			Timeout: 2,
			Err:     nil,
		},

		"reduce timeout cancel list": {
			Verb:    "GET",
			Path:    "/api/v1/pods?resourceVersion=1494416105&timeout=5s&timeoutSeconds=5",
			Timeout: 4,
			Err:     context.DeadlineExceeded,
		},

		"list with no timeout": {
			Verb:    "GET",
			Path:    "/api/v1/pods?resourceVersion=1494416105",
			Timeout: 4,
			Err:     nil,
		},

		"no reduce timeout cancel get": {
			Verb:    "GET",
			Path:    "/api/v1/pods/default/nginx?resourceVersion=1494416105&timeout=5s",
			Timeout: 2,
			Err:     nil,
		},

		"reduce timeout cancel get": {
			Verb:    "GET",
			Path:    "/api/v1/pods/default/nginx?resourceVersion=1494416105&timeout=5s",
			Timeout: 4,
			Err:     context.DeadlineExceeded,
		},

		"get with no timeout": {
			Verb:    "GET",
			Path:    "/api/v1/pods/default/nginx?resourceVersion=1494416105",
			Timeout: 4,
			Err:     nil,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
		req.RemoteAddr = "127.0.0.1"

		var ctxErr error
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			ticker := time.NewTicker(time.Duration(tc.Timeout) * time.Second)
			defer ticker.Stop()

			select {
			case <-ctx.Done():
				ctxErr = ctx.Err()
			case <-ticker.C:

			}
		})

		handler = WithRequestTimeout(handler)
		handler = filters.WithRequestInfo(handler, resolver)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if !errors.Is(ctxErr, tc.Err) {
			t.Errorf("%s: expect context cancel error %v, but got %v", k, tc.Err, ctxErr)
		}
	}
}

func TestWithListRequestSelector(t *testing.T) {
	testcases := map[string]struct {
		Verb        string
		Path        string
		HasSelector bool
		Selector    string
	}{
		"list all pods": {
			Verb:        "GET",
			Path:        "/api/v1/pods?resourceVersion=1494416105",
			HasSelector: false,
			Selector:    "",
		},
		"list pods with metadata.name": {
			Verb:        "GET",
			Path:        "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			HasSelector: false,
			Selector:    "",
		},
		"list pods with spec nodename": {
			Verb:        "GET",
			Path:        "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=spec.nodeName=test",
			HasSelector: true,
			Selector:    "spec.nodeName=test",
		},
		"list pods with label selector": {
			Verb:        "GET",
			Path:        "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&labelSelector=foo=bar",
			HasSelector: true,
			Selector:    "foo=bar",
		},
		"list pods with label selector and field selector": {
			Verb:        "GET",
			Path:        "/api/v1/namespaces/kube-system/pods?fieldSelector=spec.nodeName=test&labelSelector=foo=bar",
			HasSelector: true,
			Selector:    "foo=bar&spec.nodeName=test",
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			req.RemoteAddr = "127.0.0.1"

			var hasSelector bool
			var selector string
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := req.Context()
				selector, hasSelector = util.ListSelectorFrom(ctx)
			})

			handler = WithListRequestSelector(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if hasSelector != tc.HasSelector {
				t.Errorf("expect has selector: %v, but got %v", tc.HasSelector, hasSelector)
			}

			if selector != tc.Selector {
				t.Errorf("expect list selector %v, but got %v", tc.Selector, selector)
			}
		})
	}
}

func TestWithSaTokenSubstitute(t *testing.T) {
	//jwt token with algorithm RS256
	tenantToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJkYXRhIjpbeyJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L25hbWVzcGFjZSI6ImlvdC10ZXN0In0seyJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiZGVmYXVsdCJ9XSwiaWF0IjoxNjQ4NzkzNTI3LCJleHAiOjM3MzE1ODcxOTksImF1ZCI6IiIsImlzcyI6Imt1YmVybmV0ZXMvc2VydmljZWFjY291bnQiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.9N5ChVgM67BbUDmW2B5ziRyW5JTJYxLKPfFd57wbC-c"

	testcases := map[string]struct {
		Verb           string
		Path           string
		Token          string
		NeedSubstitute bool
	}{
		"1.no token, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/pods?resourceVersion=1494416105",
			Token:          "",
			NeedSubstitute: false,
		},
		"2.iot-test, no token, GET, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/iot-test/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			NeedSubstitute: false,
		},
		"3.iot-test, tenant token,  LIST, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/iot-test/pods?resourceVersion=1494416105",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.TYA_QK5OUN1Hmnurf27zPj-Xmh6Fxe67EzEtNI0OouElA_6FEYfuD98g2xBaUcSFZrc97ILC102gtRYX5a_IPvAgeke9WuqwoaxaA-DxMj_cUt5FUri1PEcSmtIUNM3XPgL3UebZxFn_bG_sZwYePIb7ryq4E_1XfaEA3uYO27BwuDbMxhmU6Hwsz4yKQfJDts-2SRnmG8uEc70svtgfqSBhv7EZim1S7lFY87je28sES2w-WXvWTszaUx8707QdVJjntqcxAvFUGskXQoO_hEI88xnz_-F4NX2Wiv1Mew52Srmpyh2vwTRW3TWn9_-4Lh0X9OBqnlWV0ZjElvJZig",
			NeedSubstitute: false,
		},
		"4.kube-system, GET, invalid token, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			Token:          "invalidToken",
			NeedSubstitute: false,
		},
		"5.kube-system, tenantNs iot-test001, LIST, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdDAwMSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.HrjxSSuvb-MncngvIL1rh4FnGWVZYtNfB-l8rvysP9nqGcTbKnOw5KF0SDiCvoZEK_SNYi2gJH84onsOnG7Wh7ZIjv0KbptQpVrG0dFSW6qElH_5wr2LL1_YLUalHYMmFl9jq9cD7YmXBh9B38ApuCyBIbRxOlk3QiB_ZEoSSNJX-oivHPDmoXFM2ehxaJA9cMl_i-8OSaFKaW8ptn4hN5LobI14LG2QDTNspmJqeIS5SIucl4cBJ5rRtmY6SVatGqUDsUekL-KfK0RrX4H30cTaDDJF2yLRoUvHt7fa6hDZFwvg-dh3af2aYg1_C0vGqAuLc26V12DKYPp_EIoGrg",
			NeedSubstitute: false,
		},
		"6.kube-system, WATCH, tenantNs iot-test001, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&watch=true",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdDAwMSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.HrjxSSuvb-MncngvIL1rh4FnGWVZYtNfB-l8rvysP9nqGcTbKnOw5KF0SDiCvoZEK_SNYi2gJH84onsOnG7Wh7ZIjv0KbptQpVrG0dFSW6qElH_5wr2LL1_YLUalHYMmFl9jq9cD7YmXBh9B38ApuCyBIbRxOlk3QiB_ZEoSSNJX-oivHPDmoXFM2ehxaJA9cMl_i-8OSaFKaW8ptn4hN5LobI14LG2QDTNspmJqeIS5SIucl4cBJ5rRtmY6SVatGqUDsUekL-KfK0RrX4H30cTaDDJF2yLRoUvHt7fa6hDZFwvg-dh3af2aYg1_C0vGqAuLc26V12DKYPp_EIoGrg",
			NeedSubstitute: false,
		},
		"7.kube-system, WATCH, tenantNs kube-system, need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&watch=true",
			Token:          "eyJhbGciOiJSUzI1NiIsImtpZCI6InVfTVZpZWIySUFUTzQ4NjlkM0VwTlBRb0xJOWVKUGg1ZXVzbEdaY0ZxckEifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJrdWJlLXN5c3RlbSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6a3ViZS1zeXN0ZW06ZGVmYXVsdCJ9.sFpHHg4o88Z0CBJseMBvBeP00bS5isLBmQJpAOiYs3BTkEAD63YLTnDURt0r3I9QjtcP0DZAb5wSOccGChMAFVtxMIoIoZC6Mk4FSB720kawRxFVujNFR1T7uVV_dbpEU-wsxSb9-Y4ILVknuJR9t35x6lUbRkUE9tN1wDy4DH296C3gEGNJf8sbJMERZzOckc82_BamlCzaieo1nX396KafxdQGVIgxstx88hm_rgpjDy3LA1GNsx6x2pqXdzZ8mufQt7sTljRorXUk-rNU6y9wX2RvIMO8tNiPClNkdIpgpmeQo-g7XZivpEeq3VzoeExphRbusgCtO9T9tgU64w",
			NeedSubstitute: true,
		},
	}

	resolver := newTestRequestInfoResolver()

	stopCh := make(<-chan struct{})
	tenantMgr := tenant.New("myspace", nil, stopCh)

	data := make(map[string][]byte)
	data["token"] = []byte(tenantToken)
	secret := v1.Secret{
		Data: data,
	}
	tenantMgr.SetSecret(&secret)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			req.RemoteAddr = "127.0.0.1"
			if tc.Token != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.Token))

			}

			var needSubstitute bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				rToken := req.Header.Get("Authorization")
				if rToken == fmt.Sprintf("Bearer %s", tenantToken) {
					needSubstitute = true
				}

			})

			handler = WithSaTokenSubstitute(handler, tenantMgr)
			handler = filters.WithRequestInfo(handler, resolver)

			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tc.NeedSubstitute != needSubstitute {
				t.Errorf("expect needSubsited %v, but got %v", tc.NeedSubstitute, needSubstitute)
			}

		})
	}
}

func TestWithSaTokenSubstituteTenantTokenEmpty(t *testing.T) {

	//jwt token with algorithm RS256
	tenantToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJkYXRhIjpbeyJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L25hbWVzcGFjZSI6ImlvdC10ZXN0In0seyJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiZGVmYXVsdCJ9XSwiaWF0IjoxNjQ4NzkzNTI3LCJleHAiOjM3MzE1ODcxOTksImF1ZCI6IiIsImlzcyI6Imt1YmVybmV0ZXMvc2VydmljZWFjY291bnQiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.9N5ChVgM67BbUDmW2B5ziRyW5JTJYxLKPfFd57wbC-c"
	testcases := map[string]struct {
		Verb           string
		Path           string
		Token          string
		NeedSubstitute bool
	}{
		"no token, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/pods?resourceVersion=1494416105",
			Token:          "",
			NeedSubstitute: false,
		},
		"iot-test, no token, GET, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/iot-test/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			NeedSubstitute: false,
		},
		"iot-test, tenant token,  LIST, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/iot-test/pods?resourceVersion=1494416105",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.TYA_QK5OUN1Hmnurf27zPj-Xmh6Fxe67EzEtNI0OouElA_6FEYfuD98g2xBaUcSFZrc97ILC102gtRYX5a_IPvAgeke9WuqwoaxaA-DxMj_cUt5FUri1PEcSmtIUNM3XPgL3UebZxFn_bG_sZwYePIb7ryq4E_1XfaEA3uYO27BwuDbMxhmU6Hwsz4yKQfJDts-2SRnmG8uEc70svtgfqSBhv7EZim1S7lFY87je28sES2w-WXvWTszaUx8707QdVJjntqcxAvFUGskXQoO_hEI88xnz_-F4NX2Wiv1Mew52Srmpyh2vwTRW3TWn9_-4Lh0X9OBqnlWV0ZjElvJZig",
			NeedSubstitute: false,
		},
		"kube-system, GET, invalid token, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			Token:          "invalidToken",
			NeedSubstitute: false,
		},
		"kube-system, tenantNs iot-test001, LIST, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdDAwMSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.HrjxSSuvb-MncngvIL1rh4FnGWVZYtNfB-l8rvysP9nqGcTbKnOw5KF0SDiCvoZEK_SNYi2gJH84onsOnG7Wh7ZIjv0KbptQpVrG0dFSW6qElH_5wr2LL1_YLUalHYMmFl9jq9cD7YmXBh9B38ApuCyBIbRxOlk3QiB_ZEoSSNJX-oivHPDmoXFM2ehxaJA9cMl_i-8OSaFKaW8ptn4hN5LobI14LG2QDTNspmJqeIS5SIucl4cBJ5rRtmY6SVatGqUDsUekL-KfK0RrX4H30cTaDDJF2yLRoUvHt7fa6hDZFwvg-dh3af2aYg1_C0vGqAuLc26V12DKYPp_EIoGrg",
			NeedSubstitute: false,
		},
		"kube-system, WATCH, tenantNs iot-test001, no need to substitute bearer token": {
			Verb:           "GET",
			Path:           "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&watch=true",
			Token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJpb3QtdGVzdDAwMSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJkZWZhdWx0LXRva2VuLXF3c2ZtIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQubmFtZSI6ImRlZmF1bHQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI4M2EwMzc4ZS1mY2UxLTRmZDEtOGI1NC00MTE2MjUzYzNkYWMiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6aW90LXRlc3Q6ZGVmYXVsdCJ9.HrjxSSuvb-MncngvIL1rh4FnGWVZYtNfB-l8rvysP9nqGcTbKnOw5KF0SDiCvoZEK_SNYi2gJH84onsOnG7Wh7ZIjv0KbptQpVrG0dFSW6qElH_5wr2LL1_YLUalHYMmFl9jq9cD7YmXBh9B38ApuCyBIbRxOlk3QiB_ZEoSSNJX-oivHPDmoXFM2ehxaJA9cMl_i-8OSaFKaW8ptn4hN5LobI14LG2QDTNspmJqeIS5SIucl4cBJ5rRtmY6SVatGqUDsUekL-KfK0RrX4H30cTaDDJF2yLRoUvHt7fa6hDZFwvg-dh3af2aYg1_C0vGqAuLc26V12DKYPp_EIoGrg",
			NeedSubstitute: false,
		},
	}

	resolver := newTestRequestInfoResolver()

	stopCh := make(<-chan struct{})
	tenantMgr := tenant.New("myspace", nil, stopCh)

	data := make(map[string][]byte)
	data["token"] = []byte(tenantToken)
	secret := v1.Secret{
		Data: data,
	}
	tenantMgr.SetSecret(&secret)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			req.RemoteAddr = "127.0.0.1"
			if tc.Token != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.Token))

			}

			var needSubstitute bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				rToken := req.Header.Get("Authorization")
				if rToken == fmt.Sprintf("Bearer %s", tenantToken) {
					needSubstitute = true
				}

			})

			handler = WithSaTokenSubstitute(handler, tenantMgr)
			handler = filters.WithRequestInfo(handler, resolver)

			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tc.NeedSubstitute != needSubstitute {
				t.Errorf("expect needSubsited %v, but got %v", tc.NeedSubstitute, needSubstitute)
			}

		})
	}
}

func TestIsListRequestWithNameFieldSelector(t *testing.T) {
	testcases := map[string]struct {
		Verb   string
		Path   string
		Expect bool
	}{
		"request has metadata.name fieldSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=metadata.name=test",
			Expect: true,
		},
		"request has no metadata.name fieldSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&fieldSelector=spec.nodeName=test",
			Expect: false,
		},
		"request only has labelSelector": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?resourceVersion=1494416105&labelSelector=foo=bar",
			Expect: false,
		},
		"request has both labelSelector and fieldSelector and fieldSelector has metadata.name": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?fieldSelector=metadata.name=test&labelSelector=foo=bar",
			Expect: true,
		},
		"request has both labelSelector and fieldSelector but fieldSelector has no metadata.name": {
			Verb:   "GET",
			Path:   "/api/v1/namespaces/kube-system/pods?fieldSelector=spec.nodeName=test&labelSelector=foo=bar",
			Expect: false,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
			req.RemoteAddr = "127.0.0.1"

			var isMetadataNameFieldSelector bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				isMetadataNameFieldSelector = IsListRequestWithNameFieldSelector(req)
			})

			handler = WithListRequestSelector(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if isMetadataNameFieldSelector != tc.Expect {
				t.Errorf("failed at case %s, want: %v, got: %v", k, tc.Expect, isMetadataNameFieldSelector)
			}
		})
	}
}
