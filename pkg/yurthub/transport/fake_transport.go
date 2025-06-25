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

package transport

import (
	"context"
	"net/http"
	"net/url"

	"k8s.io/client-go/kubernetes"
)

type nopRoundTrip struct {
	code int
}

func (n *nopRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     http.StatusText(n.code),
		StatusCode: n.code,
	}, nil
}

type fakeTransportManager struct {
	nop               *nopRoundTrip
	serverToClientset map[string]kubernetes.Interface
}

func NewFakeTransportManager(code int, fakeClients map[string]kubernetes.Interface) TransportManager {
	return &fakeTransportManager{
		nop:               &nopRoundTrip{code: code},
		serverToClientset: fakeClients,
	}
}

func (f *fakeTransportManager) CurrentTransport() http.RoundTripper {
	return f.nop
}

func (f *fakeTransportManager) BearerTransport() http.RoundTripper {
	return f.nop
}

func (f *fakeTransportManager) Close(_ string) {}

func (f *fakeTransportManager) GetDirectClientset(url *url.URL) kubernetes.Interface {
	if url != nil {
		return f.serverToClientset[url.String()]
	}
	return nil
}

func (f *fakeTransportManager) GetDirectClientsetAtRandom() kubernetes.Interface {
	// iterating map uses random order
	for server := range f.serverToClientset {
		return f.serverToClientset[server]
	}

	return nil
}

func (f *fakeTransportManager) ListDirectClientset() map[string]kubernetes.Interface {
	return f.serverToClientset
}

func (f *fakeTransportManager) Start(_ context.Context) {}
