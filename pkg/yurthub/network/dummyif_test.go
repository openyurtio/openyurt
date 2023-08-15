//go:build linux
// +build linux

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

package network

/*
import (
	"net"
	"testing"
)

const (
	testDummyIfName = "test-dummyif"
)

func TestEnsureDummyInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	testcases := map[string]struct {
		preparedIP string
		testIP     string
		resultIP   string
	}{
		"init ensure dummy interface": {
			testIP:   "169.254.2.1",
			resultIP: "169.254.2.1",
		},
		"ensure dummy interface after prepared": {
			preparedIP: "169.254.2.2",
			testIP:     "169.254.2.2",
			resultIP:   "169.254.2.2",
		},
		"ensure dummy interface with new ip": {
			preparedIP: "169.254.2.3",
			testIP:     "169.254.2.4",
			resultIP:   "169.254.2.4",
		},
	}

	mgr := NewDummyInterfaceController()
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			if len(tc.preparedIP) != 0 {
				err := mgr.EnsureDummyInterface(testDummyIfName, net.ParseIP(tc.preparedIP))
				if err != nil {
					t.Errorf("failed to prepare dummy interface with ip(%s), %v", tc.preparedIP, err)
				}
				ips, err := mgr.ListDummyInterface(testDummyIfName)
				if err != nil || len(ips) == 0 {
					t.Errorf("failed to prepare dummy interface(%s: %s), %v", testDummyIfName, tc.preparedIP, err)
				}
			}

			err := mgr.EnsureDummyInterface(testDummyIfName, net.ParseIP(tc.testIP))
			if err != nil {
				t.Errorf("failed to ensure dummy interface with ip(%s), %v", tc.testIP, err)
			}

			ips2, err := mgr.ListDummyInterface(testDummyIfName)
			if err != nil || len(ips2) == 0 {
				t.Errorf("failed to list dummy interface(%s), %v", testDummyIfName, err)
			}

			sameIP := false
			for _, ip := range ips2 {
				if ip.String() == tc.resultIP {
					sameIP = true
					break
				}
			}

			if !sameIP {
				t.Errorf("dummy if with ip(%s) is not ensured, addrs: %s", tc.resultIP, ips2[0].String())
			}

			// delete dummy interface
			err = mgr.DeleteDummyInterface(testDummyIfName)
			if err != nil {
				t.Errorf("failed to delte dummy interface, %v", err)
			}
		})
	}
}
*/
