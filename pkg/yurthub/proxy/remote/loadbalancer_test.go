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
	"context"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	fakeHealthChecker "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

var (
	neverStop    <-chan struct{}     = context.Background().Done()
	transportMgr transport.Interface = transport.NewFakeTransportManager(http.StatusOK, map[string]kubernetes.Interface{})
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
		results []string
	}{
		"round-robin: no backend server": {
			lbMode:  "round-robin",
			servers: map[*url.URL]bool{},
			results: []string{""},
		},
		"round-robin: one backend server": {
			lbMode: "round-robin",
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
			},
			results: []string{"127.0.0.1:8080", "127.0.0.1:8080"},
		},
		"round-robin: multiple backend servers": {
			lbMode: "round-robin",
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
			lbMode: "round-robin",
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
			lbMode: "round-robin",
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
			lbMode:  "priority",
			servers: map[*url.URL]bool{},
			results: []string{""},
		},
		"priority: one backend server": {
			lbMode: "priority",
			servers: map[*url.URL]bool{
				{Host: "127.0.0.1:8080"}: true,
			},
			results: []string{"127.0.0.1:8080", "127.0.0.1:8080"},
		},
		"priority: multiple backend servers": {
			lbMode: "priority",
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
			lbMode: "priority",
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
			lbMode: "priority",
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

			lb := NewLoadBalancer(tc.lbMode, servers, nil, transportMgr, checker, nil, neverStop)

			for _, host := range tc.results {
				strategy := lb.CurrentStrategy()
				backend := strategy.PickOne()
				if backend == nil {
					if host != "" {
						t.Errorf("expect %s, but got nil", host)
					}
				} else if backend.RemoteServer().Host != host {
					t.Errorf("expect host %s, but got %s", host, backend.RemoteServer().Host)
				}
			}
		})
	}
}
