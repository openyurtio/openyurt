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

package ip

import (
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLoopbackIP(t *testing.T) {
	lo4, err := GetLoopbackIP(false)
	if err != nil {
		t.Errorf("failed to get ipv4 loopback address: %v", err)
	}
	t.Logf("got ipv4 loopback address: %s", lo4)
	if lo4 != "127.0.0.1" {
		t.Errorf("got ipv4 loopback addr: '%s', expect: '127.0.0.1'", lo4)
	}

	lo6, err := GetLoopbackIP(true)
	if err != nil {
		t.Errorf("failed to get ipv6 loopback address: %v", err)
	}
	if lo6 != "" {
		// dual stack env
		t.Logf("got ipv6 loopback address: %s", lo6)
		if lo6 != "::1" {
			t.Errorf("got ipv6 loopback addr: '%s', expect: '::1'", lo6)
		}
	}
}

func TestRemoveDupIPs(t *testing.T) {
	tests := []struct {
		name        string
		originalIps []net.IP
		expectedIps []net.IP
	}{
		{
			"no duplication",
			[]net.IP{[]byte("1.1.1.1")},
			[]net.IP{[]byte("1.1.1.1")},
		},
		{
			"empty list",
			[]net.IP{},
			[]net.IP{},
		},
		{
			"dup list",
			[]net.IP{[]byte("1.1.1.1"), []byte("1.1.1.1")},
			[]net.IP{[]byte("1.1.1.1")},
		},
		{
			"nil list",
			nil,
			[]net.IP{},
		},
		{
			"nil ip",
			[]net.IP{[]byte("1.1.1.1"), nil},
			[]net.IP{[]byte("1.1.1.1")},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", test.name)
			{
				get := RemoveDupIPs(test.originalIps)
				if !reflect.DeepEqual(get, test.expectedIps) {
					t.Errorf("\texpect %v, but get %v", test.expectedIps, get)
				}
			}
		})
	}
}

func TestParseIPList(t *testing.T) {
	tests := []struct {
		name      string
		ipStrings []string
		ips       []net.IP
	}{
		{
			"list with formated ip",
			[]string{"1.1.1.1"},
			[]net.IP{net.IPv4(1, 1, 1, 1)},
		},
		{
			"list with not formated ip",
			[]string{"1111"},
			[]net.IP{nil},
		},
		{
			"empty list",
			[]string{},
			[]net.IP{},
		},
		{
			"nil list",
			nil,
			[]net.IP{},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", test.name)
			{
				get := ParseIPList(test.ipStrings)
				if !reflect.DeepEqual(get, test.ips) {
					t.Errorf("\texpect %v, but get %v", test.ips, get)
				}
			}
		})
	}
}

func TestSearchIPs(t *testing.T) {

	// empty ip list
	ipList := []net.IP{}
	assert.False(t, SearchIP(ipList, net.ParseIP("10.0.0.1")))

	// ip list with multiple ips
	ipList = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
	}
	assert.True(t, SearchIP(ipList, net.ParseIP("10.0.0.1")))
	assert.True(t, SearchIP(ipList, net.ParseIP("10.0.0.2")))
	assert.False(t, SearchIP(ipList, net.ParseIP("10.0.0.3")))

	// search one exiting ip
	ips := []net.IP{
		net.ParseIP("10.0.0.1"),
	}
	assert.True(t, SearchAllIP(ipList, ips))

	// search multiple existing ips
	ips = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
	}
	assert.True(t, SearchAllIP(ipList, ips))

	// search multiple existing ips with one missing ip
	ips = []net.IP{
		net.ParseIP("10.0.0.3"),
	}
	assert.False(t, SearchAllIP(ipList, ips))

	// search multiple existing ips with multiple missing ips
	ips = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.4"),
		net.ParseIP("10.0.0.3"),
		net.ParseIP("10.0.0.2"),
	}
	assert.False(t, SearchAllIP(ipList, ips))
}
