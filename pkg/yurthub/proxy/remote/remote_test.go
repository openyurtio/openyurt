/*
Copyright 2026 The OpenYurt Authors.

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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
)

type testTransportManager struct{}

func (m *testTransportManager) CurrentTransport() http.RoundTripper {
	return http.DefaultTransport
}

func (m *testTransportManager) BearerTransport() http.RoundTripper {
	return http.DefaultTransport
}

func (m *testTransportManager) Close(_ string) {}

func (m *testTransportManager) GetDirectClientset(_ *url.URL) kubernetes.Interface {
	return nil
}

func (m *testTransportManager) GetDirectClientsetAtRandom() kubernetes.Interface {
	return nil
}

func (m *testTransportManager) ListDirectClientset() map[string]kubernetes.Interface {
	return nil
}

func TestRemoteProxy_WatchUpstreamDisconnect(t *testing.T) {
	upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("expected flusher")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event1\n"))
		flusher.Flush()
		w.Write([]byte("event2\n"))
		flusher.Flush()
	})

	upstreamServer := httptest.NewServer(upstreamHandler)
	defer upstreamServer.Close()

	upstreamURL, err := url.Parse(upstreamServer.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream url: %v", err)
	}

	modifyResp := func(resp *http.Response) error {
		return nil
	}
	errHandler := func(w http.ResponseWriter, r *http.Request, err error) {}

	stopCh := make(chan struct{})
	defer close(stopCh)

	proxy, err := NewRemoteProxy(upstreamURL, modifyResp, errHandler, &testTransportManager{}, stopCh)
	if err != nil {
		t.Fatalf("failed to create remote proxy: %v", err)
	}

	proxyServer := httptest.NewServer(proxy)
	defer proxyServer.Close()

	req, err := http.NewRequest("GET", proxyServer.URL+"/api/v1/pods?watch=true", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	client := proxyServer.Client()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to perform request: %v", err)
	}
	defer resp.Body.Close()

	readDone := make(chan error, 1)
	var bodyData []byte
	go func() {
		var err error
		bodyData, err = io.ReadAll(resp.Body)
		readDone <- err
	}()

	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("unexpected error reading downstream response body: %v", err)
		}
		if string(bodyData) != "event1\nevent2\n" {
			t.Errorf("expected body 'event1\\nevent2\\n', got '%s'", string(bodyData))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for downstream response body EOF after upstream disconnect")
	}
}
