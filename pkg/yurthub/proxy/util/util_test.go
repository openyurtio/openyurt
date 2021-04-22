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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/endpoints/request"
)

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
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
			Code:        http.StatusBadRequest,
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
		UserAgent       string
		Verb            string
		Path            string
		ClientComponent string
	}{
		"kubelet request": {
			UserAgent:       "kubelet",
			Verb:            "GET",
			Path:            "/api/v1/nodes/mynode",
			ClientComponent: "kubelet",
		},

		"flanneld request": {
			UserAgent:       "flanneld/0.11.0",
			Verb:            "GET",
			Path:            "/api/v1/nodes/mynode",
			ClientComponent: "flanneld",
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
		req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
		if len(tc.UserAgent) != 0 {
			req.Header.Set("User-Agent", tc.UserAgent)
		}
		req.RemoteAddr = "127.0.0.1"

		var clientComponent string
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			clientComponent, _ = util.ClientComponentFrom(ctx)
		})

		handler = WithRequestClientComponent(handler)
		handler = filters.WithRequestInfo(handler, resolver)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if clientComponent != tc.ClientComponent {
			t.Errorf("%s: expect client component %s, but got %s", k, tc.ClientComponent, clientComponent)
		}
	}
}

func TestWithMaxInFlightLimit(t *testing.T) {
	testcases := map[int]struct {
		Verb            string
		Path            string
		ClientComponent string
		TwoManyRequests int
	}{
		10: {
			Verb:            "GET",
			Path:            "/api/v1/nodes/mynode",
			ClientComponent: "kubelet",
			TwoManyRequests: 0,
		},

		11: {
			Verb:            "GET",
			Path:            "/api/v1/nodes/mynode",
			ClientComponent: "flanneld",
			TwoManyRequests: 1,
		},
	}

	resolver := newTestRequestInfoResolver()

	for k, tc := range testcases {
		req, _ := http.NewRequest(tc.Verb, tc.Path, nil)
		req.RemoteAddr = "127.0.0.1"

		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			time.Sleep(3 * time.Second)
			w.WriteHeader(http.StatusOK)
		})

		handler = WithMaxInFlightLimit(handler, 10)
		handler = filters.WithRequestInfo(handler, resolver)

		respCodes := make([]int, k)
		var wg sync.WaitGroup
		for i := 0; i < k; i++ {
			wg.Add(1)
			go func(idx int) {
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				result := resp.Result()
				respCodes[idx] = result.StatusCode
				wg.Done()
			}(i)

		}

		wg.Wait()
		execssRequests := 0
		for i := range respCodes {
			if respCodes[i] == http.StatusTooManyRequests {
				execssRequests++
			}
		}
		if execssRequests != tc.TwoManyRequests {
			t.Errorf("%d requests: expect %d requests overflow, but got %d", k, tc.TwoManyRequests, execssRequests)
		}
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

		if ctxErr != tc.Err {
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
