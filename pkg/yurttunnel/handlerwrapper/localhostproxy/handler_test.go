/*
Copyright 2021 The OpenYurt Authors.

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

package localhostproxy

import (
	"reflect"
	"testing"
)

func TestReplaceLocalHostPorts(t *testing.T) {
	testcases := map[string]struct {
		initPorts      []string
		localhostPorts string
		resultPorts    map[string]struct{}
	}{
		"no init ports for representing configmap is added": {
			localhostPorts: "10250, 10255, 10256",
			resultPorts: map[string]struct{}{
				"10250": {},
				"10255": {},
				"10256": {},
			},
		},
		"with init ports for representing configmap is updated": {
			initPorts:      []string{"10250", "10255", "10256"},
			localhostPorts: "10250, 10255, 10256, 10257",
			resultPorts: map[string]struct{}{
				"10250": {},
				"10255": {},
				"10256": {},
				"10257": {},
			},
		},
	}

	plh := &localHostProxyMiddleware{
		localhostPorts: make(map[string]struct{}),
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			// prepare localhost ports
			for i := range tc.initPorts {
				plh.localhostPorts[tc.initPorts[i]] = struct{}{}
			}

			// run replaceLocalHostPorts
			plh.replaceLocalHostPorts(tc.localhostPorts)

			// compare replace result
			ok := reflect.DeepEqual(plh.localhostPorts, tc.resultPorts)
			if !ok {
				t.Errorf("expect localhost ports: %v, but got %v", tc.resultPorts, plh.localhostPorts)
			}

			// cleanup localhost ports
			for port := range plh.localhostPorts {
				delete(plh.localhostPorts, port)
			}
		})
	}
}
