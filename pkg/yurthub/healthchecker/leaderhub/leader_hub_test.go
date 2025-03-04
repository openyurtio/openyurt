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

package leaderhub

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeaderHubHealthChecker(t *testing.T) {
	testcases := map[string]struct {
		servers                              []*url.URL
		updatedServers                       []*url.URL
		pingFunc                             func(*url.URL) bool
		expectedIsHealthy                    bool
		expectedBackendIsHealthy             map[*url.URL]bool
		expectedBackendIsHealthyAfterUpdated map[*url.URL]bool
		healthyServerFound                   bool
	}{
		"all servers are unhealthy": {
			servers: []*url.URL{
				{Host: "127.0.0.1:8081"},
				{Host: "127.0.0.1:8082"},
				{Host: "127.0.0.1:8083"},
			},
			pingFunc: func(server *url.URL) bool {
				return false
			},
			expectedIsHealthy: false,
			expectedBackendIsHealthy: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: false,
				{Host: "127.0.0.1:8083"}: false,
			},
			healthyServerFound: false,
		},
		"all servers are healthy": {
			servers: []*url.URL{
				{Host: "127.0.0.1:8081"},
				{Host: "127.0.0.1:8082"},
				{Host: "127.0.0.1:8083"},
			},
			pingFunc: func(server *url.URL) bool {
				return true
			},
			expectedIsHealthy: true,
			expectedBackendIsHealthy: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: true,
				{Host: "127.0.0.1:8082"}: true,
				{Host: "127.0.0.1:8083"}: true,
			},
			healthyServerFound: true,
		},
		"a part of servers are unhealthy": {
			servers: []*url.URL{
				{Host: "127.0.0.1:8081"},
				{Host: "127.0.0.1:8082"},
				{Host: "127.0.0.1:8083"},
			},
			pingFunc: func(server *url.URL) bool {
				if server.Host == "127.0.0.1:8081" ||
					server.Host == "127.0.0.1:8082" {
					return false
				}
				return true
			},
			expectedIsHealthy: true,
			expectedBackendIsHealthy: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: false,
				{Host: "127.0.0.1:8083"}: true,
			},
			healthyServerFound: true,
		},
		"no servers are prepared": {
			servers: []*url.URL{},
			pingFunc: func(server *url.URL) bool {
				if server.Host == "127.0.0.1:8081" ||
					server.Host == "127.0.0.1:8082" {
					return false
				}
				return true
			},
			expectedIsHealthy: false,
			expectedBackendIsHealthy: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: false,
			},
			healthyServerFound: false,
		},
		"wait and updated servers": {
			servers: []*url.URL{
				{Host: "127.0.0.1:8081"},
				{Host: "127.0.0.1:8082"},
			},
			updatedServers: []*url.URL{
				{Host: "127.0.0.1:8082"},
				{Host: "127.0.0.1:8083"},
			},
			pingFunc: func(server *url.URL) bool {
				return true
			},
			expectedIsHealthy: true,
			expectedBackendIsHealthy: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: true,
				{Host: "127.0.0.1:8082"}: true,
				{Host: "127.0.0.1:8083"}: false,
			},
			expectedBackendIsHealthyAfterUpdated: map[*url.URL]bool{
				{Host: "127.0.0.1:8081"}: false,
				{Host: "127.0.0.1:8082"}: true,
				{Host: "127.0.0.1:8083"}: true,
			},
			healthyServerFound: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			stopCh := make(chan struct{})
			hc := NewLeaderHubHealthChecker(2*time.Second, tc.pingFunc, stopCh)
			hc.UpdateBackends(tc.servers)

			assert.Equal(t, hc.IsHealthy(), tc.expectedIsHealthy, "IsHealthy result is not equal")
			assert.Equal(t, hc.PickOneHealthyBackend() != nil, tc.healthyServerFound, "PickOneHealthyBackend result is not equal")
			for u, isHealthy := range tc.expectedBackendIsHealthy {
				assert.Equal(t, hc.BackendIsHealthy(u), isHealthy, "BackendIsHealthy result is not equal")
			}

			if len(tc.updatedServers) != 0 {
				time.Sleep(5 * time.Second)
				hc.UpdateBackends(tc.updatedServers)
				for u, isHealthy := range tc.expectedBackendIsHealthyAfterUpdated {
					assert.Equal(t, hc.BackendIsHealthy(u), isHealthy, "BackendIsHealthy result is not equal after updated")
				}
			}
			close(stopCh)
		})
	}
}
