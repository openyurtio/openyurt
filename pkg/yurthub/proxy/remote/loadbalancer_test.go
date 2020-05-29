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

package remote

import (
	"net/url"
	"testing"

	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
)

type PickBackend struct {
	DeltaRequestsCnt int
	ReturnServer     string
}

func TestRrLoadBalancerAlgo(t *testing.T) {
	testcases := map[string]struct {
		Servers      []string
		PickBackends []PickBackend
	}{
		"no backend servers": {
			Servers: []string{},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: ""},
			},
		},

		"one backend server": {
			Servers: []string{"http://127.0.0.1:8080"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
			},
		},

		"multi backend server": {
			Servers: []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081", "http://127.0.0.1:8082"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 2, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 3, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 5, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 5, ReturnServer: "http://127.0.0.1:8080"},
			},
		},
	}

	checker := healthchecker.NewFakeChecker(true, map[string]int{})
	for k, tc := range testcases {
		backends := make([]*RemoteProxy, len(tc.Servers))
		for i := range tc.Servers {
			u, _ := url.Parse(tc.Servers[i])
			backends[i] = &RemoteProxy{
				remoteServer: u,
				checker:      checker,
			}
		}

		rr := &rrLoadBalancerAlgo{
			backends: backends,
		}

		for i := range tc.PickBackends {
			var b *RemoteProxy
			for j := 0; j < tc.PickBackends[i].DeltaRequestsCnt; j++ {
				b = rr.PickOne()
			}

			if len(tc.PickBackends[i].ReturnServer) == 0 {
				if b != nil {
					t.Errorf("%s rr lb pick: expect no backend server, but got %s", k, b.remoteServer.String())
				}
			} else {
				if b == nil {
					t.Errorf("%s rr lb pick: expect backend server: %s, but got no backend server", k, tc.PickBackends[i].ReturnServer)
				} else if b.remoteServer.String() != tc.PickBackends[i].ReturnServer {
					t.Errorf("%s rr lb pick(round %d): expect backend server: %s, but got %s", k, i+1, tc.PickBackends[i].ReturnServer, b.remoteServer.String())
				}
			}
		}
	}
}

func TestRrLoadBalancerAlgoWithReverseHealthy(t *testing.T) {
	testcases := map[string]struct {
		Servers      []string
		PickBackends []PickBackend
	}{
		"multi backend server": {
			Servers: []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081", "http://127.0.0.1:8082"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
			},
		},
	}

	checker := healthchecker.NewFakeChecker(true, map[string]int{
		"http://127.0.0.1:8080": 1,
		"http://127.0.0.1:8081": 2,
	})
	for k, tc := range testcases {
		backends := make([]*RemoteProxy, len(tc.Servers))
		for i := range tc.Servers {
			u, _ := url.Parse(tc.Servers[i])
			backends[i] = &RemoteProxy{
				remoteServer: u,
				checker:      checker,
			}
		}

		rr := &rrLoadBalancerAlgo{
			backends: backends,
		}

		for i := range tc.PickBackends {
			var b *RemoteProxy
			for j := 0; j < tc.PickBackends[i].DeltaRequestsCnt; j++ {
				b = rr.PickOne()
			}

			if len(tc.PickBackends[i].ReturnServer) == 0 {
				if b != nil {
					t.Errorf("%s rr lb pick: expect no backend server, but got %s", k, b.remoteServer.String())
				}
			} else {
				if b == nil {
					t.Errorf("%s rr lb pick(round %d): expect backend server: %s, but got no backend server", k, i+1, tc.PickBackends[i].ReturnServer)
				} else if b.remoteServer.String() != tc.PickBackends[i].ReturnServer {
					t.Errorf("%s rr lb pick(round %d): expect backend server: %s, but got %s", k, i+1, tc.PickBackends[i].ReturnServer, b.remoteServer.String())
				}
			}
		}
	}
}

func TestPriorityLoadBalancerAlgo(t *testing.T) {
	testcases := map[string]struct {
		Servers      []string
		PickBackends []PickBackend
	}{
		"no backend servers": {
			Servers: []string{},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: ""},
			},
		},

		"one backend server": {
			Servers: []string{"http://127.0.0.1:8080"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
			},
		},

		"multi backend server": {
			Servers: []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081", "http://127.0.0.1:8082"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 2, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 3, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 4, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 5, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 5, ReturnServer: "http://127.0.0.1:8080"},
			},
		},
	}

	checker := healthchecker.NewFakeChecker(true, map[string]int{})
	for k, tc := range testcases {
		backends := make([]*RemoteProxy, len(tc.Servers))
		for i := range tc.Servers {
			u, _ := url.Parse(tc.Servers[i])
			backends[i] = &RemoteProxy{
				remoteServer: u,
				checker:      checker,
			}
		}

		rr := &priorityLoadBalancerAlgo{
			backends: backends,
		}

		for i := range tc.PickBackends {
			var b *RemoteProxy
			for j := 0; j < tc.PickBackends[i].DeltaRequestsCnt; j++ {
				b = rr.PickOne()
			}

			if len(tc.PickBackends[i].ReturnServer) == 0 {
				if b != nil {
					t.Errorf("%s priority lb pick: expect no backend server, but got %s", k, b.remoteServer.String())
				}
			} else {
				if b == nil {
					t.Errorf("%s priority lb pick: expect backend server: %s, but got no backend server", k, tc.PickBackends[i].ReturnServer)
				} else if b.remoteServer.String() != tc.PickBackends[i].ReturnServer {
					t.Errorf("%s priority lb pick(round %d): expect backend server: %s, but got %s", k, i+1, tc.PickBackends[i].ReturnServer, b.remoteServer.String())
				}
			}
		}
	}
}

func TestPriorityLoadBalancerAlgoWithReverseHealthy(t *testing.T) {
	testcases := map[string]struct {
		Servers      []string
		PickBackends []PickBackend
	}{
		"multi backend server": {
			Servers: []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081", "http://127.0.0.1:8082"},
			PickBackends: []PickBackend{
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8080"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8081"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 1, ReturnServer: "http://127.0.0.1:8082"},
				{DeltaRequestsCnt: 2, ReturnServer: "http://127.0.0.1:8082"},
			},
		},
	}

	checker := healthchecker.NewFakeChecker(true, map[string]int{
		"http://127.0.0.1:8080": 2,
		"http://127.0.0.1:8081": 3})
	for k, tc := range testcases {
		backends := make([]*RemoteProxy, len(tc.Servers))
		for i := range tc.Servers {
			u, _ := url.Parse(tc.Servers[i])
			backends[i] = &RemoteProxy{
				remoteServer: u,
				checker:      checker,
			}
		}

		rr := &priorityLoadBalancerAlgo{
			backends: backends,
		}

		for i := range tc.PickBackends {
			var b *RemoteProxy
			for j := 0; j < tc.PickBackends[i].DeltaRequestsCnt; j++ {
				b = rr.PickOne()
			}

			if len(tc.PickBackends[i].ReturnServer) == 0 {
				if b != nil {
					t.Errorf("%s priority lb pick: expect no backend server, but got %s", k, b.remoteServer.String())
				}
			} else {
				if b == nil {
					t.Errorf("%s priority lb pick: expect backend server: %s, but got no backend server", k, tc.PickBackends[i].ReturnServer)
				} else if b.remoteServer.String() != tc.PickBackends[i].ReturnServer {
					t.Errorf("%s priority lb pick(round %d): expect backend server: %s, but got %s", k, i+1, tc.PickBackends[i].ReturnServer, b.remoteServer.String())
				}
			}
		}
	}
}
