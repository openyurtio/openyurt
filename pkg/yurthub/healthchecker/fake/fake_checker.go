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

package fake

import (
	"net/url"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

type FakeChecker struct {
	servers map[*url.URL]bool
}

// BackendHealthyStatus returns healthy status of server
func (fc *FakeChecker) BackendIsHealthy(server *url.URL) bool {
	if server != nil {
		for s, healthy := range fc.servers {
			if s.Host == server.Host {
				return healthy
			}
		}
	}
	return false
}

func (fc *FakeChecker) IsHealthy() bool {
	for _, isHealthy := range fc.servers {
		if isHealthy {
			return true
		}
	}
	return false
}

func (fc *FakeChecker) RenewKubeletLeaseTime() {
}

func (fc *FakeChecker) PickOneHealthyBackend() *url.URL {
	for u, isHealthy := range fc.servers {
		if isHealthy {
			return u
		}
	}

	return nil
}

func (fc *FakeChecker) UpdateBackends(servers []*url.URL) {
	serverMap := make(map[*url.URL]bool, len(servers))
	for i := range servers {
		serverMap[servers[i]] = false
	}

	fc.servers = serverMap
}

func (fc *FakeChecker) ListServerHosts() sets.Set[string] {
	hosts := sets.New[string]()
	for server := range fc.servers {
		hosts.Insert(server.Host)
	}

	return hosts
}

// NewFakeChecker creates a fake checker
func NewFakeChecker(servers map[*url.URL]bool) healthchecker.Interface {
	return &FakeChecker{
		servers: servers,
	}
}
