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
	healthy  bool
	settings map[string]int
}

// BackendHealthyStatus returns healthy status of server
func (fc *fakeChecker) BackendIsHealthy(server *url.URL) bool {
	s := server.String()
	if _, ok := fc.settings[s]; !ok {
		return fc.healthy
	}

	if fc.settings[s] < 0 {
		return fc.healthy
	}

	if fc.settings[s] == 0 {
		return !fc.healthy
	}

	fc.settings[s] = fc.settings[s] - 1
	return fc.healthy
}

func (fc *fakeChecker) IsHealthy() bool {
	return fc.healthy
}

func (fc *fakeChecker) RenewKubeletLeaseTime() {
}

func (fc *fakeChecker) PickOneHealthyBackend() *url.URL {
	for server := range fc.settings {
		if fc.healthy {
			if u, err := url.Parse(server); err == nil {
				return u
			}
		}
	}

	return nil
}

func (fc *fakeChecker) UpdateServers(servers []*url.URL) {
	// do nothing
}

// NewFakeChecker creates a fake checker
func NewFakeChecker(healthy bool, settings map[string]int) healthchecker.Interface {
	return &fakeChecker{
		settings: settings,
		healthy:  healthy,
	}
}
