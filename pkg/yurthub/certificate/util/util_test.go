/*
Copyright 2023 The OpenYurt Authors.

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

package util

import (
	"net"
	"net/url"
	"testing"
	"time"
)

// TestFindActiveRemoteServer 测试 findActiveRemoteServer 函数
func TestFindActiveRemoteServer(t *testing.T) {
	mockDialer := func(network, address string, timeout time.Duration) (net.Conn, error) {
		if address == "active.server:80" {
			return nil, nil
		}
		return nil, net.UnknownNetworkError("mock error")
	}

	servers := []*url.URL{
		{Host: "inactive.server:80"},
		{Host: "active.server:80"},
	}

	active := FindActiveRemoteServer(mockDialer, servers)

	if active.Host != "active.server:80" {
		t.Errorf("Expected active.server:80, got %v", active.Host)
	}
}
