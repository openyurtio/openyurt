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
	"encoding/base64"
	"io"
	"io/ioutil"
	"testing"
)

func TestDualReader(t *testing.T) {
	src := []byte("hello, world")
	rb := bytes.NewBuffer(src)
	rc := ioutil.NopCloser(rb)
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
	rc := ioutil.NopCloser(rb)
	drc, prc := NewDualReadCloser(nil, rc, true)
	rc = drc
	dst := make([]byte, len(src))

	if err := prc.Close(); err != nil {
		t.Errorf("prc.Close failed %v", err)
	}

	if n, err := io.ReadFull(rc, dst); n != 0 || err != io.ErrClosedPipe {
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
