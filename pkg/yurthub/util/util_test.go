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
	"io/ioutil"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"
	"os"
	"testing"
)

var (
	testCaCert = []byte(`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJANXr+UzRFq4TMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwIBcNMTcwNDI2MjMyNzMyWhgPMjExNzA0
MDIyMzI3MzJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTEwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAqvbkN4RShH1rL37JFp4fZPnn0JUhVWWsrP8NOomJ
pXdBDUMGWuEQIsZ1Gf9JrCQLu6ooRyHSKRFpAVbMQ3ABJwIDAQABo1AwTjAdBgNV
HQ4EFgQUEGBc6YYheEZ/5MhwqSUYYPYRj2MwHwYDVR0jBBgwFoAUEGBc6YYheEZ/
5MhwqSUYYPYRj2MwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAIyNmznk
5dgJY52FppEEcfQRdS5k4XFPc22SHPcz77AHf5oWZ1WG9VezOZZPp8NCiFDDlDL8
yma33a5eMyTjLD8=
-----END CERTIFICATE-----`)
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

func TestKeyFunc(t *testing.T) {
	type expectData struct {
		err bool
		key string
	}
	tests := []struct {
		desc     string
		comp     string
		resource string
		ns       string
		name     string
		result   expectData
	}{
		{
			desc:   "no resource",
			comp:   "kubelet",
			result: expectData{err: true},
		},
		{
			desc:     "no comp",
			resource: "pods",
			result:   expectData{err: true},
		},
		{
			desc:     "with comp and resource",
			comp:     "kubelet",
			resource: "pods",
			result:   expectData{key: "kubelet/pods"},
		},
		{
			desc:     "with comp resource and ns",
			comp:     "kubelet",
			resource: "pods",
			ns:       "default",
			result:   expectData{key: "kubelet/pods/default"},
		},
		{
			desc:     "with comp resource and name",
			comp:     "kubelet",
			resource: "pods",
			name:     "mypod1",
			result:   expectData{key: "kubelet/pods/mypod1"},
		},
		{
			desc:     "with all items",
			comp:     "kubelet",
			resource: "pods",
			ns:       "default",
			name:     "mypod1",
			result:   expectData{key: "kubelet/pods/default/mypod1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			key, err := KeyFunc(tt.comp, tt.resource, tt.ns, tt.name)
			if tt.result.err {
				if err == nil {
					t.Errorf("expect error returned, but not error")
				}
			} else {
				if err != nil {
					t.Errorf("Got error %v", err)
				}

				if key != tt.result.key {
					t.Errorf("%s Expect, but got %s", tt.result.key, key)
				}
			}
		})
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
	dir, err := ioutil.TempDir("", "yurthub-util-file-exist")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	testExistFile := dir + "test.txt"
	if err = ioutil.WriteFile(testExistFile, nil, 0600); err != nil {
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

func TestCreateKubeConfigFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "yurthub-util-kubeconfig")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	testKubeConfigPath := dir + "kube.config"
	type args struct {
		kubeClientConfig *rest.Config
		kubeconfigPath   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no input return error", args{}, true},
		{"no legal input return error", args{kubeClientConfig: new(rest.Config)}, true},
		{"legal input with no error", args{kubeClientConfig: &rest.Config{
			TLSClientConfig: rest.TLSClientConfig{
				CAData:   testCaCert,
				CertFile: "/tmp/not-exist-path",
				KeyFile:  "/tmp/not-exist-path",
			},
		}, kubeconfigPath: testKubeConfigPath}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CreateKubeConfigFile(tt.args.kubeClientConfig, tt.args.kubeconfigPath); (err != nil) != tt.wantErr {
				t.Errorf("CreateKubeConfigFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadKubeConfig(t *testing.T) {
	type args struct {
		kubeconfig string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no config path, return default config", args{}, false},
		{"illegal config path, return error", args{"/tmp/not-exist-config-path"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadKubeConfig(tt.args.kubeconfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadKubeConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestLoadRESTClientConfig(t *testing.T) {
	dir, err := ioutil.TempDir("", "yurthub-util-kubeconfig")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	testKubeConfigPath := dir + "kube.config"
	if err = ioutil.WriteFile(testKubeConfigPath, []byte(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURmVENDQXllZ0F3SUJBZ0lVRkJsNGdVb3FaRFAvd1VKRG4zNy9WSjl1cEQwd0RRWUpLb1pJaHZjTkFRRUYKQlFBd2ZqRUxNQWtHQTFVRUJoTUNSMEl4RHpBTkJnTlZCQWdNQmt4dmJtUnZiakVQTUEwR0ExVUVCd3dHVEc5dQpaRzl1TVJnd0ZnWURWUVFLREE5SGJHOWlZV3dnVTJWamRYSnBkSGt4RmpBVUJnTlZCQXNNRFVsVUlFUmxjR0Z5CmRHMWxiblF4R3pBWkJnTlZCQU1NRW5SbGMzUXRZMlZ5ZEdsbWFXTmhkR1V0TURBZUZ3MHlNREF6TURJeE9UTTMKTURCYUZ3MHlNVEF6TURJeE9UTTNNREJhTUlHSU1Rc3dDUVlEVlFRR0V3SlZVekVUTUJFR0ExVUVDQk1LUTJGcwphV1p2Y201cFlURVdNQlFHQTFVRUJ4TU5VMkZ1SUVaeVlXNWphWE5qYnpFZE1Cc0dBMVVFQ2hNVVJYaGhiWEJzClpTQkRiMjF3WVc1NUxDQk1URU14RXpBUkJnTlZCQXNUQ2s5d1pYSmhkR2x2Ym5NeEdEQVdCZ05WQkFNVEQzZDMKZHk1bGVHRnRjR3hsTG1OdmJUQ0NBU0l3RFFZSktvWklodmNOQVFFQkJRQURnZ0VQQURDQ0FRb0NnZ0VCQU1pUgpETnBtd1RJQ0ZyK1AxNmZLRFZqYk5DelNqV3ErTVR1OHZBZlM2R3JMcEJUVUVlKzZ6VnF4VXphL2ZaZW54bzhPCnVjVjJKVFV2NUo0bmtUL3ZHNlFtL21Ub1ZKNHZRekxRNWpSMnc3di83Y2Yzb1dDd1RBS1VhZmdvNi9HYTk1Z24KbFFCMytGZDhzeTk2emZGci83d0RTTVBQdWVSNWtTRmF4K2NFZDMwd3d2NU83dFdqMHJvMW1yeExzc0Jsd1BhUgpabHpra3Z4QllUeldDcUtac1drdFFsWGNpcWxGU29zMHVhN3V2d3FLTjVDVHhmQy94b3lNeHg5a2ZabTdCelBOClpEcVlNRncySGlXZEVpTHpJNGpqK0doMEQ1dDQ3dG52bHBVTWloY1g5eDBqUDYvK2huZmNROEdBUDJqUi9CWFkKNVlaUlJZNzBMaUNYUGV2bFJBRUNBd0VBQWFPQnFUQ0JwakFPQmdOVkhROEJBZjhFQkFNQ0JhQXdIUVlEVlIwbApCQll3RkFZSUt3WUJCUVVIQXdFR0NDc0dBUVVGQndNQ01Bd0dBMVVkRXdFQi93UUNNQUF3SFFZRFZSME9CQllFCkZPb2lFK2toN2dHRHB5eDBLWnVDYzFscmxUUktNQjhHQTFVZEl3UVlNQmFBRk50b3N2R2xwRFVzYjlKd2NSY1gKcTM3TDUyVlRNQ2NHQTFVZEVRUWdNQjZDQzJWNFlXMXdiR1V1WTI5dGdnOTNkM2N1WlhoaGJYQnNaUzVqYjIwdwpEUVlKS29aSWh2Y05BUUVGQlFBRFFRQXc2bXhRT05BRDJzaXZmeklmMWVERmQ2TFU3YUUrTW5rZGxFUWpqUENpCnRsVUlURkl1TzNYYXZJU3VwUDZWOXdFMGIxd1RGMXBUbFZXQXJmLzBZUVhzCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
    server: https://127.0.0.1
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: kubernetes-admin
  name: kubernetes-admin@kubernetes
current-context: kubernetes-admin@kubernetes
kind: Config
preferences: {}
users:
- name: kubernetes-admin
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNSekNDQWZHZ0F3SUJBZ0lKQUxNYjdlY01JazNNTUEwR0NTcUdTSWIzRFFFQkN3VUFNSDR4Q3pBSkJnTlYKQkFZVEFrZENNUTh3RFFZRFZRUUlEQVpNYjI1a2IyNHhEekFOQmdOVkJBY01Ca3h2Ym1SdmJqRVlNQllHQTFVRQpDZ3dQUjJ4dlltRnNJRk5sWTNWeWFYUjVNUll3RkFZRFZRUUxEQTFKVkNCRVpYQmhjblJ0Wlc1ME1Sc3dHUVlEClZRUUREQkowWlhOMExXTmxjblJwWm1sallYUmxMVEF3SUJjTk1UY3dOREkyTWpNeU5qVXlXaGdQTWpFeE56QTAKTURJeU16STJOVEphTUg0eEN6QUpCZ05WQkFZVEFrZENNUTh3RFFZRFZRUUlEQVpNYjI1a2IyNHhEekFOQmdOVgpCQWNNQmt4dmJtUnZiakVZTUJZR0ExVUVDZ3dQUjJ4dlltRnNJRk5sWTNWeWFYUjVNUll3RkFZRFZRUUxEQTFKClZDQkVaWEJoY25SdFpXNTBNUnN3R1FZRFZRUUREQkowWlhOMExXTmxjblJwWm1sallYUmxMVEF3WERBTkJna3EKaGtpRzl3MEJBUUVGQUFOTEFEQklBa0VBdEJNYTdOV3B2M0JWbEtUQ1BHTy9MRXNndUtxV0hCdEt6d2VNWTJDVgp0QUwxclFtOTEzaHVoeEY5dythaTc2S1EzTUhLNUlWbkxKallZQTVNelAySDVRSURBUUFCbzFBd1RqQWRCZ05WCkhRNEVGZ1FVMjJpeThhV2tOU3h2MG5CeEZ4ZXJmc3ZuWlZNd0h3WURWUjBqQkJnd0ZvQVUyMml5OGFXa05TeHYKMG5CeEZ4ZXJmc3ZuWlZNd0RBWURWUjBUQkFVd0F3RUIvekFOQmdrcWhraUc5dzBCQVFzRkFBTkJBRU9lZkdiVgpOY0h4a2xhVzA2dzZPQllKUHdwSWhDVm96QzFxZHhHWDFkZzhWa0VLempPempncVZEMzBtNTlPRm1TbEJtSHNsCm5rVkE2d3lPU0RZQmYzbz0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlCVXdJQkFEQU5CZ2txaGtpRzl3MEJBUUVGQUFTQ0FUMHdnZ0U1QWdFQUFrRUF0Qk1hN05XcHYzQlZsS1RDClBHTy9MRXNndUtxV0hCdEt6d2VNWTJDVnRBTDFyUW05MTNodWh4Rjl3K2FpNzZLUTNNSEs1SVZuTEpqWVlBNU0KelAySDVRSURBUUFCQWtBUzlCZlhhYjNPS3BLM2JJZ05OeXArRFFKS3JablRKNFErT2pzcWtwWHZObHRQSm9zZgpHOEdzaUt1L3ZBdDRIR3FJM2VVNzdOdlJJK21MNE1uSFJtWEJBaUVBM3FNNEZBdEtTUkJiY0p6UHh4TEVVU3dnClhTQ2Nvc0NrdGJrWHZwWXJTMzBDSVFEUER4Z3Fsd0RFSlEwdUt1SGtaSTM4L1NQV1dxZlVta2Vjd2xicFhBQksKaVFJZ1pYMDhEQThWZnZjQTUvWGoxWmpkZXk5RlZZNlBPTFhlbjZSUGlhYkU5N1VDSUNwNmVVVzdodCsyamphcgplMzVFbHRDUkNqb2VqUkhUdU45VEMwdUNvVmlwQWlBWGFKSXgvUTQ3dkd3aXc2WThLWHNOVTZ5NTRnVGJPU3hYCjU0THpITmsvK1E9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=`), 0600); err != nil {
		t.Fatalf("failed to write test kube config to tmp dir. err:%v", err)
	}
	type args struct {
		kubeconfig string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no config path, return error", args{}, true},
		{"illegal config path, return error", args{"/tmp/not-exist-config-path"}, true},
		{"legal config path, no error", args{testKubeConfigPath}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadRESTClientConfig(tt.args.kubeconfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadRESTClientConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestLoadKubeletRestClientConfig(t *testing.T) {
	dir, err := ioutil.TempDir("", "yurthub-util-load-kubelet-rest-client-config")
	if err != nil {
		t.Fatalf("Unable to create the test directory %q: %v", dir, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", dir, err)
		}
	}()
	legalKubeConfigFile := dir + "legal.config"
	if err = ioutil.WriteFile(legalKubeConfigFile, []byte(`-----BEGIN CERTIFICATE-----
MIICRzCCAfGgAwIBAgIJALMb7ecMIk3MMA0GCSqGSIb3DQEBCwUAMH4xCzAJBgNV
BAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNVBAcMBkxvbmRvbjEYMBYGA1UE
CgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1JVCBEZXBhcnRtZW50MRswGQYD
VQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwIBcNMTcwNDI2MjMyNjUyWhgPMjExNzA0
MDIyMzI2NTJaMH4xCzAJBgNVBAYTAkdCMQ8wDQYDVQQIDAZMb25kb24xDzANBgNV
BAcMBkxvbmRvbjEYMBYGA1UECgwPR2xvYmFsIFNlY3VyaXR5MRYwFAYDVQQLDA1J
VCBEZXBhcnRtZW50MRswGQYDVQQDDBJ0ZXN0LWNlcnRpZmljYXRlLTAwXDANBgkq
hkiG9w0BAQEFAANLADBIAkEAtBMa7NWpv3BVlKTCPGO/LEsguKqWHBtKzweMY2CV
tAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5MzP2H5QIDAQABo1AwTjAdBgNV
HQ4EFgQU22iy8aWkNSxv0nBxFxerfsvnZVMwHwYDVR0jBBgwFoAU22iy8aWkNSxv
0nBxFxerfsvnZVMwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAANBAEOefGbV
NcHxklaW06w6OBYJPwpIhCVozC1qdxGX1dg8VkEKzjOzjgqVD30m59OFmSlBmHsl
nkVA6wyOSDYBf3o=
-----END CERTIFICATE-----
-----BEGIN RSA PRIVATE KEY-----
MIIBUwIBADANBgkqhkiG9w0BAQEFAASCAT0wggE5AgEAAkEAtBMa7NWpv3BVlKTC
PGO/LEsguKqWHBtKzweMY2CVtAL1rQm913huhxF9w+ai76KQ3MHK5IVnLJjYYA5M
zP2H5QIDAQABAkAS9BfXab3OKpK3bIgNNyp+DQJKrZnTJ4Q+OjsqkpXvNltPJosf
G8GsiKu/vAt4HGqI3eU77NvRI+mL4MnHRmXBAiEA3qM4FAtKSRBbcJzPxxLEUSwg
XSCcosCktbkXvpYrS30CIQDPDxgqlwDEJQ0uKuHkZI38/SPWWqfUmkecwlbpXABK
iQIgZX08DA8VfvcA5/Xj1Zjdey9FVY6POLXen6RPiabE97UCICp6eUW7ht+2jjar
e35EltCRCjoejRHTuN9TC0uCoVipAiAXaJIx/Q47vGwiw6Y8KXsNU6y54gTbOSxX
54LzHNk/+Q==
-----END RSA PRIVATE KEY-----`), 0600); err != nil {
		t.Fatalf("write legal kube config to file. err:%v", err)
	}
	illegalKubeConfigFile := dir + "illegal.config"
	if err = ioutil.WriteFile(illegalKubeConfigFile, []byte(`something illegal`), 0600); err != nil {
		t.Fatalf("write illegal kube config to file. err:%v", err)
	}
	type args struct {
		healthyServer         *url.URL
		kubeletRootCAFilePath string
		kubeletPairFilePath   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no input, want error", args{}, true},
		{"no root ca, want error", args{healthyServer: new(url.URL)}, true},
		{"no legal root ca, want error", args{healthyServer: new(url.URL), kubeletRootCAFilePath: illegalKubeConfigFile}, true},
		{"legal root ca, no error", args{healthyServer: new(url.URL), kubeletPairFilePath: legalKubeConfigFile}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadKubeletRestClientConfig(tt.args.healthyServer, tt.args.kubeletRootCAFilePath, tt.args.kubeletPairFilePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadKubeletRestClientConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
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

func TestIsKubeletLeaseReq(t *testing.T) {
	type args struct {
		req *http.Request
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"null request", args{new(http.Request)}, false},
		{"not kubelet request", args{
			new(http.Request).WithContext(WithClientComponent(context.Background(), "not-kubelet")),
		}, false},
		{"no request info in request", args{
			new(http.Request).
				WithContext(WithClientComponent(context.Background(), "kubelet")),
		}, false},
		{"request source is not leases", args{
			new(http.Request).
				WithContext(apirequest.WithRequestInfo(WithClientComponent(context.Background(), "kubelet"), &apirequest.RequestInfo{Resource: "not-leases"})),
		}, false},
		{"valid request", args{
			new(http.Request).
				WithContext(apirequest.WithRequestInfo(WithClientComponent(context.Background(), "kubelet"), &apirequest.RequestInfo{Resource: "leases"})),
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKubeletLeaseReq(tt.args.req); got != tt.want {
				t.Errorf("IsKubeletLeaseReq() = %v, want %v", got, tt.want)
			}
		})
	}
}
