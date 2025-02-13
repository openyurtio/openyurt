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

package cloudapiserver

import (
	"net/url"

	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

type fakeChecker struct {
	servers map[*url.URL]bool
}

// BackendHealthyStatus returns healthy status of server
func (fc *fakeChecker) BackendIsHealthy(server *url.URL) bool {
	if server != nil {
		for s, healthy := range fc.servers {
			if s.Host == server.Host {
				return healthy
			}
		}
	}
	return false
}

func (fc *fakeChecker) IsHealthy() bool {
	for _, isHealthy := range fc.servers {
		if isHealthy {
			return true
		}
	}
	return false
}

func (fc *fakeChecker) RenewKubeletLeaseTime() {
}

func (fc *fakeChecker) PickOneHealthyBackend() *url.URL {
	for u, isHealthy := range fc.servers {
		if isHealthy {
			return u
		}
	}

	return nil
}

func (fc *fakeChecker) UpdateServers(servers []*url.URL) {
	// do nothing
}

// NewFakeChecker creates a fake checker
func NewFakeChecker(servers map[*url.URL]bool) healthchecker.Interface {
	return &fakeChecker{
		servers: servers,
	}
}
