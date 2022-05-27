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
	"testing"
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
