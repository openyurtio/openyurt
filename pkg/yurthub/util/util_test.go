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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	apirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestContext(t *testing.T) {
	ctx := context.Background()
	reqContentType, ok := ReqContentTypeFrom(ctx)
	if ok || reqContentType != "" {
		t.Errorf("want clean context, got value %s, ok:%v", reqContentType, ok)
		return
	}
	testReqContentType := "testReqContentType"
	ctxWithReqContentType := WithReqContentType(ctx, testReqContentType)
	reqContentType, ok = ReqContentTypeFrom(ctxWithReqContentType)
	if !ok || reqContentType != testReqContentType {
		t.Errorf("want reqContentType, got value %s, ok:%v", reqContentType, ok)
		return
	}

	respContentType, ok := RespContentTypeFrom(ctx)
	if ok || respContentType != "" {
		t.Errorf("want clean context, got value %s, ok:%v", respContentType, ok)
		return
	}
	testRespContentType := "testRespContentType"
	ctxWithRespContentType := WithRespContentType(ctx, testRespContentType)
	respContentType, ok = RespContentTypeFrom(ctxWithRespContentType)
	if !ok || respContentType != testRespContentType {
		t.Errorf("want respContentType, got value %s, ok:%v", respContentType, ok)
		return
	}

	clientComponent, ok := ClientComponentFrom(ctx)
	if ok || clientComponent != "" {
		t.Errorf("want clean context, got value %s, ok:%v", clientComponent, ok)
		return
	}
	testClientComponent := "testClientComponent"
	ctxWithClientComponent := WithClientComponent(ctx, testClientComponent)
	clientComponent, ok = ClientComponentFrom(ctxWithClientComponent)
	if !ok || clientComponent != testClientComponent {
		t.Errorf("want clientComponent, got value %s, ok:%v", clientComponent, ok)
		return
	}

	reqCanCacheFrom, ok := ReqCanCacheFrom(ctx)
	if ok {
		t.Errorf("want clean context, got value %v, ok:%v", reqCanCacheFrom, ok)
		return
	}
	testReqCanCacheFrom := true
	ctxWithReqCanCache := WithReqCanCache(ctx, testReqCanCacheFrom)
	reqCanCacheFrom, ok = ReqCanCacheFrom(ctxWithReqCanCache)
	if !ok || reqCanCacheFrom != testReqCanCacheFrom {
		t.Errorf("reqCanCacheFrom, got value %v, ok:%v", reqCanCacheFrom, ok)
		return
	}

	listSelectorFrom, ok := ListSelectorFrom(ctx)
	if ok || listSelectorFrom != "" {
		t.Errorf("want clean context, got value %v, ok:%v", listSelectorFrom, ok)
		return
	}
	testListSelectorFrom := "testListSelectorFrom"
	ctxWithListSelector := WithListSelector(ctx, testListSelectorFrom)
	listSelectorFrom, ok = ListSelectorFrom(ctxWithListSelector)
	if !ok || listSelectorFrom != testListSelectorFrom {
		t.Errorf("want listSelectorFrom, got value %v, ok:%v", listSelectorFrom, ok)
		return
	}
}

func TestDualReader(t *testing.T) {
	src := []byte("hello, world")
	rb := bytes.NewBuffer(src)
	rc := io.NopCloser(rb)
	drc, prc := NewDualReadCloser(nil, rc, true)
	rc = drc
	dst1 := make([]byte, len(src))
	dst2 := make([]byte, len(src))

	go func() {
		if n2, err := io.ReadFull(prc, dst2); err != nil || n2 != len(src) {
			t.Errorf("ReadFull(prc, dst2) = %d, %v; want %d, nil", n2, err, len(src))
		}
	}()

	if n1, err := io.ReadFull(rc, dst1); err != nil || n1 != len(src) {
		t.Fatalf("ReadFull(rc, dst1) = %d, %v; want %d, nil", n1, err, len(src))
	}

	if !bytes.Equal(dst1, src) {
		t.Errorf("rc: bytes read = %q want %q", dst1, src)
	}

	if !bytes.Equal(dst2, src) {
		t.Errorf("nr: bytes read = %q want %q", dst2, src)
	}

	if n, err := rc.Read(dst1); n != 0 || err != io.EOF {
		t.Errorf("rc.Read at EOF = %d, %v want 0, EOF", n, err)
	}

	if err := rc.Close(); err != nil {
		t.Errorf("rc.Close failed %v", err)
	}

	if n, err := prc.Read(dst1); n != 0 || err != io.EOF {
		t.Errorf("nr.Read at EOF = %d, %v want 0, EOF", n, err)
	}
}

func TestDualReaderByPreClose(t *testing.T) {
	src := []byte("hello, world")
	rb := bytes.NewBuffer(src)
	rc := io.NopCloser(rb)
	drc, prc := NewDualReadCloser(nil, rc, true)
	rc = drc
	dst := make([]byte, len(src))

	if err := prc.Close(); err != nil {
		t.Errorf("prc.Close failed %v", err)
	}

	if n, err := io.ReadFull(rc, dst); n != 0 || !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("closed dualReadCloser: ReadFull(r, dst) = %d, %v; want 0, EPIPE", n, err)
	}
}

func TestSplitKey(t *testing.T) {
	type expectData struct {
		comp     string
		resource string
		ns       string
		name     string
	}
	tests := []struct {
		desc   string
		key    string
		result expectData
	}{
		{
			desc:   "no key",
			key:    "",
			result: expectData{},
		},
		{
			desc: "comp split",
			key:  "kubelet",
			result: expectData{
				comp: "kubelet",
			},
		},
		{
			desc: "comp and resource split",
			key:  "kubelet/nodes",
			result: expectData{
				comp:     "kubelet",
				resource: "nodes",
			},
		},
		{
			desc: "comp resource and name split",
			key:  "kubelet/nodes/mynode1",
			result: expectData{
				comp:     "kubelet",
				resource: "nodes",
				name:     "mynode1",
			},
		},
		{
			desc: "all items split",
			key:  "kubelet/pods/default/mypod1",
			result: expectData{
				comp:     "kubelet",
				resource: "pods",
				ns:       "default",
				name:     "mypod1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			comp, resource, ns, name := SplitKey(tt.key)
			if comp != tt.result.comp ||
				resource != tt.result.resource ||
				ns != tt.result.ns ||
				name != tt.result.name {
				t.Errorf("%v expect, but go %s/%s/%s/%s", tt.result, comp, resource, ns, name)
			}
		})
	}
}

func TestParseTenantNs(t *testing.T) {

	testCases := map[string]string{
		"a":                       "",
		"openyurt:tenant:myspace": "myspace",
	}

	for k, v := range testCases {

		ns := ParseTenantNs(k)
		if v != ns {
			t.Errorf("%s is not equal to %s", v, ns)
		}

	}

	token := "ZXlKaGJHY2lPaUpTVXpJMU5pSXNJbXRwWkNJNkluVmZUVlpwWldJeVNVRlVUelE0Tmpsa00wVndUbEJSYjB4Sk9XVktVR2cxWlhWemJFZGFZMFp4Y2tFaWZRLmV5SnBjM01pT2lKcmRXSmxjbTVsZEdWekwzTmxjblpwWTJWaFkyTnZkVzUwSWl3aWEzVmlaWEp1WlhSbGN5NXBieTl6WlhKMmFXTmxZV05qYjNWdWRDOXVZVzFsYzNCaFkyVWlPaUpwYjNRdGRHVnpkQ0lzSW10MVltVnlibVYwWlhNdWFXOHZjMlZ5ZG1salpXRmpZMjkxYm5RdmMyVmpjbVYwTG01aGJXVWlPaUprWldaaGRXeDBMWFJ2YTJWdUxYRjNjMlp0SWl3aWEzVmlaWEp1WlhSbGN5NXBieTl6WlhKMmFXTmxZV05qYjNWdWRDOXpaWEoyYVdObExXRmpZMjkxYm5RdWJtRnRaU0k2SW1SbFptRjFiSFFpTENKcmRXSmxjbTVsZEdWekxtbHZMM05sY25acFkyVmhZMk52ZFc1MEwzTmxjblpwWTJVdFlXTmpiM1Z1ZEM1MWFXUWlPaUk0TTJFd016YzRaUzFtWTJVeExUUm1aREV0T0dJMU5DMDBNVEUyTWpVell6TmtZV01pTENKemRXSWlPaUp6ZVhOMFpXMDZjMlZ5ZG1salpXRmpZMjkxYm5RNmFXOTBMWFJsYzNRNlpHVm1ZWFZzZENKOS5QM2xuc3NWSTZvVUg2U19CX2thYU1QWmV5Vm8xM2xCQU50aGVvb3ByY1ZnQWlIWXpNOVdBcUFPTi12c2h5RTBBcUFHTFl3Q1FsY0FReXhKNDZqbEd0TXJxUlhpRWIyMldobXVtRkswc3NNTGJkbHBOWmJjNzc5WmxoeXUyVDJnRTlKSExFMHUyUFkwQm5sQUlQZmtGYzZPZk9veklybDBGZUxGWklFY1MzQi1yTlUwYUZDekJZNEpsMThYdUpKOEhubHA4N3V1Q2FlLUZzWHJWajFIZUd4MWw4S2JzZVJwSkFrN0Q0aklPNDFndXRlSHV5MnE3SldHLUwyWWZ0VG1peWdEb2pqMlhFTkEyTkxrRXFLbG5NQ3BlSjFwUl82UjRKZ21OaTUzLWktTE5mTVNGWXNnckNMUWNNTkhiZkg1MEpBOXp0cHd1Y2xmWUl3WjBPZkdPOWc="

	out, _ := base64.StdEncoding.DecodeString(token)
	t.Logf("token: %s", string(out))
}

func TestIsSupportedLBMode(t *testing.T) {
	type args struct {
		lbMode string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"lb mode rr", args{"rr"}, true},
		{"lb mode priority", args{"priority"}, true},
		{"no lb mode", args{""}, false},
		{"illegal lb mode", args{"illegal-mode"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedLBMode(tt.args.lbMode); got != tt.want {
				t.Errorf("IsSupportedLBMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSupportedWorkingMode(t *testing.T) {
	type args struct {
		workingMode WorkingMode
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"working mode cloud", args{WorkingModeCloud}, true},
		{"working mode edge", args{WorkingModeEdge}, true},
		{"no working mode", args{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedWorkingMode(tt.args.workingMode); got != tt.want {
				t.Errorf("IsSupportedWorkingMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	dir, err := os.MkdirTemp("", "yurthub-util-file-exist")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	testExistFile := dir + "test.txt"
	if err = os.WriteFile(testExistFile, nil, 0600); err != nil {
		t.Fatalf("Unable to create the test file %q: %v", testExistFile, err)
	}
	testNotExistFile := dir + "not-exist-test.txt"
	type args struct {
		filename string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"file exists", args{testExistFile}, true, false},
		{"file not exists", args{testNotExistFile}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FileExists(tt.args.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FileExists() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTenantNsFromOrgs(t *testing.T) {
	type args struct {
		orgs []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"no orgs", args{}, ""},
		{"no tenant in orgs", args{[]string{"org1", "org2"}}, ""},
		{"there is tenant in orgs", args{[]string{"org1", "org2", "openyurt:tenant:special-tenant"}}, "special-tenant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseTenantNsFromOrgs(tt.args.orgs); got != tt.want {
				t.Errorf("ParseTenantNsFromOrgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseBearerToken(t *testing.T) {
	type args struct {
		token string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"no token", args{}, ""},
		{"no bearer token", args{"some-token"}, ""},
		{"there is bearer token", args{"Bearer token-content"}, "token-content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseBearerToken(tt.args.token); got != tt.want {
				t.Errorf("ParseBearerToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewGZipReaderCloser(t *testing.T) {
	gzipHeader := http.Header{}
	gzipHeader.Set("Content-Encoding", "gzip")
	noGZipHeader := http.Header{}
	noGZipHeader.Set("Some-Header", "value")
	type args struct {
		header http.Header
		src    []byte
		req    *http.Request
		caller string
	}
	tests := []struct {
		name  string
		args  args
		want1 bool
	}{
		{"no header", args{}, false},
		{"no gzip header", args{header: noGZipHeader}, false},
		{"valid gzip header", args{header: gzipHeader, src: []byte("Hello World"), req: &http.Request{URL: &url.URL{Host: "http://127.0.0.1"}, Header: gzipHeader}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := bytes.NewBuffer(tt.args.src)
			rc := io.NopCloser(rb)
			_, got1 := NewGZipReaderCloser(tt.args.header, rc, tt.args.req, tt.args.caller)
			if got1 != tt.want1 {
				t.Errorf("NewGZipReaderCloser() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestReqInfoString(t *testing.T) {
	type args struct {
		info *apirequest.RequestInfo
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"nil request", args{}, ""},
		{"valid request", args{
			&apirequest.RequestInfo{
				Verb:     http.MethodGet,
				Resource: "services",
				Path:     "/api/v1/namespaces/default/services?watch=true",
			},
		}, "GET services for /api/v1/namespaces/default/services?watch=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReqInfoString(tt.args.info); got != tt.want {
				t.Errorf("ReqInfoString() = %v, want %v", got, tt.want)
			}
		})
	}
}
