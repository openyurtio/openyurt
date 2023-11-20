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
	"time"
)

type dialer func(network, address string, timeout time.Duration) (net.Conn, error)

func FindActiveRemoteServer(dial dialer, servers []*url.URL) *url.URL {
	if len(servers) == 0 {
		return nil
	} else if len(servers) == 1 {
		return servers[0]
	}

	for i := range servers {
		_, err := dial("tcp", servers[i].Host, 5*time.Second)
		if err == nil {
			return servers[i]
		}
	}

	return servers[0]
}
