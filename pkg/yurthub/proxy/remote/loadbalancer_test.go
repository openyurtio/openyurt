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

package remote

import (
	"net/http"
	"net/url"
	"sort"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

func sortURLs(urls []*url.URL) {
	sort.Slice(urls, func(i, j int) bool {
		return urls[i].Host < urls[j].Host
	})
}

func TestLoadBalancingStrategy(t *testing.T) {
	testcases := map[string]struct {
		lbMode  string
		servers map[*url.URL]bool
		req     []*http.Request
		results []string
	}{
		"round-robin: no backend server": {
			lbMode:  roundRobinStrategy,
			servers: map[*url.URL]bool{},
			results: []string{""},
		},
		"round-robin: one backend server": {
			lbMode: roundRobinStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
			},
			results: []string{"127.0.0.1:8080", "127.0.0.1:8080"},
		},
		"round-robin: multiple backend servers": {
			lbMode: roundRobinStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
				{Host: "127.0.0.1:8081"}: true,
				{Host: "127.0.0.1:8082"}: true,
				{Host: "127.0.0.1:8083"}: true,
			},
			results: []string{
				"127.0.0.1:8080",
				"127.0.0.1:8081",
				"127.0.0.1:8082",
				"127.0.0.1:8083",
				"127.0.0.1:8080",
			},
		},
		"round-robin: multiple backend servers with unhealthy server": {
			lbMode: roundRobinStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: true,
			},
			results: []string{
				"127.0.0.1:8080",
				"127.0.0.1:8082",
				"127.0.0.1:8080",
			},
		},
		"round-robin: all of backend servers are unhealthy": {
			lbMode: roundRobinStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: false,
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: false,
			},
			results: []string{
				"",
				"",
				"",
				"",
			},
		},
		"priority: no backend server": {
			lbMode:  priorityStrategy,
			servers: map[*url.URL]bool{},
			results: []string{""},
		},
		"priority: one backend server": {
			lbMode: priorityStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
			},
			results: []string{"127.0.0.1:8080", "127.0.0.1:8080"},
		},
		"priority: multiple backend servers": {
			lbMode: priorityStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
				{Host: "127.0.0.1:8081"}: true,
				{Host: "127.0.0.1:8082"}: true,
			},
			results: []string{
				"127.0.0.1:8080",
				"127.0.0.1:8080",
				"127.0.0.1:8080",
				"127.0.0.1:8080",
			},
		},
		"priority: multiple backend servers with unhealthy server": {
			lbMode: priorityStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: false,
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: true,
			},
			results: []string{
				"127.0.0.1:8082",
				"127.0.0.1:8082",
				"127.0.0.1:8082",
			},
		},
		"priority: all of backend servers are unhealthy": {
			lbMode: priorityStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: false,
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: false,
			},
			results: []string{
				"",
				"",
				"",
				"",
			},
		},
		"consistent-hashing: no backend server": {
			lbMode:  consistentHashingStrategy,
			servers: map[*url.URL]bool{},
			results: []string{""},
		},
		"consistent-hashing: one backend server": {
			lbMode: consistentHashingStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
			},
			results: []string{"127.0.0.1:8080", "127.0.0.1:8080"},
		},
		"consistent-hashing: multiple backend servers": {
			lbMode: consistentHashingStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}:   true,
				{Host: "192.168.0.1:8081"}: true,
				{Host: "10.0.0.1:8082"}:    true,
			},
			req: []*http.Request{
				{
					Header: map[string][]string{
						"User-Agent": {"user-agent-1"},
					},
					RequestURI: "/path-1",
				},
				{
					Header: map[string][]string{
						"User-Agent": {"Chrome/109.0.0.0"},
					},
					RequestURI: "/resource-foobarbaz",
				},
				{
					Header: map[string][]string{
						"User-Agent": {"CoreDNS/1.6.0"},
					},
					RequestURI: "/foobarbaz-resource",
				},
				{
					Header: map[string][]string{
						"User-Agent": {"curl"},
					},
					RequestURI: "/baz-resource",
				},
			},
			results: []string{
				"127.0.0.1:8080",
				"192.168.0.1:8081",
				"127.0.0.1:8080",
				"10.0.0.1:8082",
			},
		},
		"consistent-hashing: multiple backend servers with unhealthy server": {
			lbMode: consistentHashingStrategy,
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}:   false,
				{Host: "192.168.0.1:8081"}: false,
				{Host: "10.0.0.1:8082"}:    true,
			},
			req: []*http.Request{
				{
					Header:     map[string][]string{"User-Agent": {"user-agent-1"}},
					RequestURI: "/path-1",
				},
				{
					Header:     map[string][]string{"User-Agent": {"Chrome/109.0.0.0"}},
					RequestURI: "/resource-foobarbaz",
				},
				{
					Header:     map[string][]string{"User-Agent": {"CoreDNS/1.6.0"}},
					RequestURI: "/foobarbaz-resource",
				},
				{
					Header:     map[string][]string{"User-Agent": {"curl"}},
					RequestURI: "/baz-resource",
				},
			},
			results: []string{
				"10.0.0.1:8082",
				"10.0.0.1:8082",
				"10.0.0.1:8082",
				"10.0.0.1:8082",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			checker := fakeHealthChecker.NewFakeChecker(tc.servers)
			servers := make([]*url.URL, 0, len(tc.servers))
			for server := range tc.servers {
				servers = append(servers, server)
			}
			sortURLs(servers)
			klog.Infof("servers: %+v", servers)

			var transportMgr transport.TransportManager = transport.NewFakeTransportManager(
				http.StatusOK,
				map[string]kubernetes.Interface{},
			)

			transportMgr.Start(t.Context())
			lb := NewLoadBalancer(tc.lbMode, servers, nil, transportMgr, checker, nil)

			for i, host := range tc.results {
				strategy := lb.CurrentStrategy()
				req := &http.Request{}
				if tc.req != nil {
					req = tc.req[i]
				}
				backend := strategy.PickOne(req)
				if backend == nil {
					if host != "" {
						t.Errorf("expect %s, but got nil", host)
					}
				} else if backend.RemoteServer().Host != host {
					t.Errorf("expect host %s for req %d, but got %s", host, i, backend.RemoteServer().Host)
				}
			}
		})
	}
}

func TestGetHash(t *testing.T) {
	testCases := map[string]struct {
		key      string
		expected uint32
	}{
		"empty key": {
			key:      "",
			expected: 2166136261,
		},
		"normal key": {
			key:      "10.0.0.1:8080",
			expected: 1080829289,
		},
	}

	for k, tc := range testCases {
		t.Run(k, func(t *testing.T) {
			hash := getHash(tc.key)
			if hash != tc.expected {
				t.Errorf("expect hash %d, but got %d", tc.expected, hash)
			}
		})
	}
}
