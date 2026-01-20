/*
Copyright 2025 The OpenYurt Authors.

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

package remote

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type mockCacheManager struct {
	canCacheForFunc   func(req *http.Request) bool
	cacheResponseFunc func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error
	deleteKindForFunc func(gvr schema.GroupVersionResource) error
}

func (m *mockCacheManager) CacheResponse(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error {
	if m.cacheResponseFunc != nil {
		return m.cacheResponseFunc(req, prc, stopCh)
	}
	_, _ = io.Copy(io.Discard, prc)
	return nil
}

func (m *mockCacheManager) QueryCache(req *http.Request) (runtime.Object, error) {
	return nil, nil
}

func (m *mockCacheManager) CanCacheFor(req *http.Request) bool {
	if m.canCacheForFunc != nil {
		return m.canCacheForFunc(req)
	}
	return true
}

func (m *mockCacheManager) DeleteKindFor(gvr schema.GroupVersionResource) error {
	if m.deleteKindForFunc != nil {
		return m.deleteKindForFunc(gvr)
	}
	return nil
}

func (m *mockCacheManager) QueryCacheResult() cachemanager.CacheResult {
	return cachemanager.CacheResult{}
}

type mockFilterFinder struct {
	findResponseFilterFunc func(req *http.Request) (filter.ResponseFilter, bool)
}

func (m *mockFilterFinder) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
	if m.findResponseFilterFunc != nil {
		return m.findResponseFilterFunc(req)
	}
	return nil, false
}

func (m *mockFilterFinder) FindObjectFilter(req *http.Request) (filter.ObjectFilter, bool) {
	return nil, false
}

func (m *mockFilterFinder) HasSynced() bool {
	return true
}

type mockResponseFilter struct {
	name       string
	filterFunc func(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error)
}

func (m *mockResponseFilter) Name() string {
	return m.name
}

func (m *mockResponseFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	if m.filterFunc != nil {
		return m.filterFunc(req, rc, stopCh)
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return 0, nil, err
	}
	return len(data), io.NopCloser(bytes.NewBuffer(data)), nil
}

func createTestRequest(verb, resource, apiGroup, apiVersion string) *http.Request {
	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	ctx := req.Context()

	info := &apirequest.RequestInfo{
		Verb:       verb,
		Resource:   resource,
		APIGroup:   apiGroup,
		APIVersion: apiVersion,
	}
	ctx = apirequest.WithRequestInfo(ctx, info)
	ctx = hubutil.WithReqContentType(ctx, "application/json")

	return req.WithContext(ctx)
}

func TestModifyResponse_NilResponse(t *testing.T) {
	lb := &LoadBalancer{}

	err := lb.modifyResponse(nil)
	if err != nil {
		t.Errorf("Expected no error for nil response, got: %v", err)
	}
}

func TestModifyResponse_NilRequest(t *testing.T) {
	lb := &LoadBalancer{}
	resp := &http.Response{}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Expected no error for response with nil request, got: %v", err)
	}
}

func TestModifyResponse_WatchRequest_AddsChunkedHeader(t *testing.T) {
	lb := &LoadBalancer{}

	req := createTestRequest("watch", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("test data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	transferEncoding := resp.Header.Get("Transfer-Encoding")
	if transferEncoding != "chunked" {
		t.Errorf("Expected Transfer-Encoding: chunked for watch request, got: %s", transferEncoding)
	}
}

func TestModifyResponse_WatchRequest_PreservesExistingChunkedHeader(t *testing.T) {
	lb := &LoadBalancer{}

	req := createTestRequest("watch", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Transfer-Encoding": []string{"chunked"},
		},
		Body:    io.NopCloser(bytes.NewBufferString("test data")),
		Request: req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	values := resp.Header.Values("Transfer-Encoding")
	if len(values) != 1 || values[0] != "chunked" {
		t.Errorf("Expected single Transfer-Encoding: chunked, got: %v", values)
	}
}

func TestModifyResponse_SuccessfulResponse_WithFilter(t *testing.T) {
	filteredData := []byte("filtered data")
	filterCalled := false

	mockFilter := &mockResponseFilter{
		name: "test-filter",
		filterFunc: func(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
			filterCalled = true
			_, _ = io.ReadAll(rc)
			return len(filteredData), io.NopCloser(bytes.NewBuffer(filteredData)), nil
		},
	}

	mockFinder := &mockFilterFinder{
		findResponseFilterFunc: func(req *http.Request) (filter.ResponseFilter, bool) {
			return mockFilter, true
		},
	}

	lb := &LoadBalancer{
		filterFinder: mockFinder,
	}

	req := createTestRequest("list", "pods", "", "v1")
	originalData := []byte("original data")
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{},
		Body:          io.NopCloser(bytes.NewBuffer(originalData)),
		ContentLength: int64(len(originalData)),
		Request:       req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !filterCalled {
		t.Error("Expected filter to be called")
	}

	expectedLength := int64(len(filteredData))
	if resp.ContentLength != expectedLength {
		t.Errorf("Expected ContentLength %d, got %d", expectedLength, resp.ContentLength)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if !bytes.Equal(body, filteredData) {
		t.Errorf("Expected body %q, got %q", filteredData, body)
	}
}

func TestModifyResponse_SuccessfulResponse_WithFilterError(t *testing.T) {
	expectedErr := fmt.Errorf("filter error")

	mockFilter := &mockResponseFilter{
		name: "test-filter",
		filterFunc: func(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
			return 0, nil, expectedErr
		},
	}

	mockFinder := &mockFilterFinder{
		findResponseFilterFunc: func(req *http.Request) (filter.ResponseFilter, bool) {
			return mockFilter, true
		},
	}

	lb := &LoadBalancer{
		filterFinder: mockFinder,
	}

	req := createTestRequest("list", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("test data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err == nil {
		t.Error("Expected error from filter, got nil")
	}
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestModifyResponse_SuccessfulResponse_WithCache(t *testing.T) {
	canCacheCalled := false

	mockCache := &mockCacheManager{
		canCacheForFunc: func(req *http.Request) bool {
			canCacheCalled = true
			return true
		},
		cacheResponseFunc: func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error {
			_, _ = io.ReadAll(prc)
			return nil
		},
	}

	lb := &LoadBalancer{
		localCacheMgr: mockCache,
		stopCh:        make(chan struct{}),
	}

	req := createTestRequest("list", "pods", "", "v1")
	testData := []byte("test data for caching")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBuffer(testData)),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !canCacheCalled {
		t.Error("Expected CanCacheFor to be called")
	}
}

func TestModifyResponse_SuccessfulResponse_CacheNotEnabled(t *testing.T) {
	canCacheCalled := false

	mockCache := &mockCacheManager{
		canCacheForFunc: func(req *http.Request) bool {
			canCacheCalled = true
			return false
		},
	}

	lb := &LoadBalancer{
		localCacheMgr: mockCache,
	}

	req := createTestRequest("list", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("test data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !canCacheCalled {
		t.Error("Expected CanCacheFor to be called")
	}
}

func TestModifyResponse_SuccessfulResponse_NoFilterNoCache(t *testing.T) {
	lb := &LoadBalancer{}

	req := createTestRequest("list", "pods", "", "v1")
	testData := []byte("test data")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBuffer(testData)),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if !bytes.Equal(body, testData) {
		t.Errorf("Expected body %q, got %q", testData, body)
	}
}

func TestModifyResponse_404Response_ListRequest_DeletesKind(t *testing.T) {
	deleteKindCalled := false
	var deletedGVR schema.GroupVersionResource

	mockCache := &mockCacheManager{
		deleteKindForFunc: func(gvr schema.GroupVersionResource) error {
			deleteKindCalled = true
			deletedGVR = gvr
			return nil
		},
	}

	lb := &LoadBalancer{
		localCacheMgr: mockCache,
	}

	req := createTestRequest("list", "customresources", "custom.example.com", "v1")
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("not found")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !deleteKindCalled {
		t.Error("Expected DeleteKindFor to be called for 404 list response")
	}

	expectedGVR := schema.GroupVersionResource{
		Group:    "custom.example.com",
		Version:  "v1",
		Resource: "customresources",
	}
	if deletedGVR != expectedGVR {
		t.Errorf("Expected GVR %v, got %v", expectedGVR, deletedGVR)
	}
}

func TestModifyResponse_404Response_GetRequest_DoesNotDeleteKind(t *testing.T) {
	deleteKindCalled := false

	mockCache := &mockCacheManager{
		deleteKindForFunc: func(gvr schema.GroupVersionResource) error {
			deleteKindCalled = true
			return nil
		},
	}

	lb := &LoadBalancer{
		localCacheMgr: mockCache,
	}

	req := createTestRequest("get", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("not found")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if deleteKindCalled {
		t.Error("DeleteKindFor should not be called for non-list requests")
	}
}

func TestModifyResponse_404Response_NoCacheManager(t *testing.T) {
	lb := &LoadBalancer{}

	req := createTestRequest("list", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("not found")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestModifyResponse_ErrorResponse_NoProcessing(t *testing.T) {
	lb := &LoadBalancer{}

	testCases := []struct {
		name       string
		statusCode int
	}{
		{"BadRequest", http.StatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized},
		{"Forbidden", http.StatusForbidden},
		{"InternalServerError", http.StatusInternalServerError},
		{"ServiceUnavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := createTestRequest("list", "pods", "", "v1")
			testData := []byte("error message")
			resp := &http.Response{
				StatusCode: tc.statusCode,
				Header:     http.Header{},
				Body:       io.NopCloser(bytes.NewBuffer(testData)),
				Request:    req,
			}

			err := lb.modifyResponse(resp)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			if !bytes.Equal(body, testData) {
				t.Errorf("Expected body %q, got %q", testData, body)
			}
		})
	}
}

func TestModifyResponse_ContentTypeHandling(t *testing.T) {
	lb := &LoadBalancer{}

	testCases := []struct {
		name                string
		requestContentType  string
		responseContentType string
	}{
		{
			name:                "Response has content type",
			requestContentType:  "application/json",
			responseContentType: "application/json; charset=utf-8",
		},
		{
			name:                "Response missing content type, uses request",
			requestContentType:  "application/vnd.kubernetes.protobuf",
			responseContentType: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := createTestRequest("list", "pods", "", "v1")
			ctx := hubutil.WithReqContentType(req.Context(), tc.requestContentType)
			req = req.WithContext(ctx)

			headers := http.Header{}
			if tc.responseContentType != "" {
				headers.Set("Content-Type", tc.responseContentType)
			}

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     headers,
				Body:       io.NopCloser(bytes.NewBufferString("test data")),
				Request:    req,
			}

			err := lb.modifyResponse(resp)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestModifyResponse_PartialContentResponse(t *testing.T) {
	lb := &LoadBalancer{}

	req := createTestRequest("get", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusPartialContent,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("partial data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestModifyResponse_NoRequestInfo(t *testing.T) {
	lb := &LoadBalancer{}

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("test data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestModifyResponse_FilterAndCache_Integration(t *testing.T) {
	filteredData := []byte("filtered and cached data")
	filterCalled := false

	mockFilter := &mockResponseFilter{
		name: "test-filter",
		filterFunc: func(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
			filterCalled = true
			_, _ = io.ReadAll(rc)
			return len(filteredData), io.NopCloser(bytes.NewBuffer(filteredData)), nil
		},
	}

	mockFinder := &mockFilterFinder{
		findResponseFilterFunc: func(req *http.Request) (filter.ResponseFilter, bool) {
			return mockFilter, true
		},
	}

	mockCache := &mockCacheManager{
		canCacheForFunc: func(req *http.Request) bool {
			return true
		},
		cacheResponseFunc: func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error {
			_, _ = io.ReadAll(prc)
			return nil
		},
	}

	lb := &LoadBalancer{
		filterFinder:  mockFinder,
		localCacheMgr: mockCache,
		stopCh:        make(chan struct{}),
	}

	req := createTestRequest("list", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("original data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !filterCalled {
		t.Error("Expected filter to be called")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if !bytes.Equal(body, filteredData) {
		t.Errorf("Expected body %q, got %q", filteredData, body)
	}
}

func TestModifyResponse_StopChannelContext(t *testing.T) {
	stopCh := make(chan struct{})
	close(stopCh)

	lb := &LoadBalancer{
		stopCh: stopCh,
	}

	req := createTestRequest("list", "pods", "", "v1")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewBufferString("test data")),
		Request:    req,
	}

	err := lb.modifyResponse(resp)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
