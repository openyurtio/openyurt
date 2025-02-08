/*
Copyright 2025 The OpenYurt Authors.

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

package leaderhub

import (
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

type leaderHubHealthChecker struct {
	serverMutex   sync.Mutex
	statusMutex   sync.RWMutex
	servers       []*url.URL
	status        map[string]bool
	checkInterval time.Duration
	pingFunc      func(*url.URL) bool
}

func NewLeaderHubHealthChecker(checkerInterval time.Duration, pingFunc func(*url.URL) bool, stopCh <-chan struct{}) healthchecker.Interface {
	if pingFunc == nil {
		pingFunc = pingServer
	}

	hc := &leaderHubHealthChecker{
		servers:       make([]*url.URL, 0),
		status:        make(map[string]bool),
		checkInterval: checkerInterval,
		pingFunc:      pingFunc,
	}
	go hc.startHealthCheck(stopCh)

	return hc
}

func (hc *leaderHubHealthChecker) startHealthCheck(stopCh <-chan struct{}) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.checkServers()
		case <-stopCh:
			return
		}
	}
}

func (hc *leaderHubHealthChecker) IsHealthy() bool {
	hc.statusMutex.RLock()
	defer hc.statusMutex.RUnlock()

	for _, healthy := range hc.status {
		if healthy {
			return true
		}
	}

	return false
}

func (hc *leaderHubHealthChecker) BackendIsHealthy(server *url.URL) bool {
	hc.statusMutex.RLock()
	defer hc.statusMutex.RUnlock()

	healthy, exists := hc.status[server.String()]
	return exists && healthy
}

func (hc *leaderHubHealthChecker) PickOneHealthyBackend() *url.URL {
	hc.statusMutex.RLock()
	defer hc.statusMutex.RUnlock()
	for server, healthy := range hc.status {
		if healthy {
			if u, err := url.Parse(server); err == nil {
				return u
			}
		}
	}

	return nil
}

func (hc *leaderHubHealthChecker) UpdateBackends(servers []*url.URL) {
	hc.serverMutex.Lock()
	defer hc.serverMutex.Unlock()
	newStatus := make(map[string]bool)
	for _, server := range servers {
		newStatus[server.String()] = hc.pingFunc(server)
	}

	hc.statusMutex.Lock()
	hc.status = newStatus
	hc.servers = servers
	hc.statusMutex.Unlock()
}

func (hc *leaderHubHealthChecker) RenewKubeletLeaseTime() {
	// do nothing
}

func (hc *leaderHubHealthChecker) checkServers() {
	hc.serverMutex.Lock()
	defer hc.serverMutex.Unlock()

	if len(hc.servers) == 0 {
		return
	}
	newStatus := make(map[string]bool)
	for _, server := range hc.servers {
		newStatus[server.String()] = hc.pingFunc(server)
	}

	hc.statusMutex.Lock()
	hc.status = newStatus
	hc.statusMutex.Unlock()
}

func pingServer(server *url.URL) bool {
	if server != nil {
		conn, err := net.DialTimeout("tcp", server.Host, 2*time.Second)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
	return false
}
