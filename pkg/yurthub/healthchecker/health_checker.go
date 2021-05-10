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

package healthchecker

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

const (
	heartbeatFrequency = 10 * time.Second
)

// HealthChecker is an interface for checking healthy stats of server
type HealthChecker interface {
	IsHealthy(server *url.URL) bool
	Run()
}

type setNodeLease func(*coordinationv1.Lease) error
type getNodeLease func() *coordinationv1.Lease

type healthCheckerManager struct {
	remoteServers     []*url.URL
	checkers          map[string]*checker
	latestLease       *coordinationv1.Lease
	sw                cachemanager.StorageWrapper
	remoteServerIndex int
	stopCh            <-chan struct{}
}

type checker struct {
	sync.RWMutex
	remoteServer     *url.URL
	clusterHealthy   bool
	healthyThreshold int
	healthyCnt       int
	lastTime         time.Time
	nodeLease        NodeLease
	getLastNodeLease getNodeLease
	setLastNodeLease setNodeLease
	onFailureFunc    func(string)
}

// NewHealthChecker create an HealthChecker for servers
func NewHealthChecker(cfg *config.YurtHubConfiguration, tp transport.Interface, sw cachemanager.StorageWrapper, stopCh <-chan struct{}) (HealthChecker, error) {
	if len(cfg.RemoteServers) == 0 {
		return nil, fmt.Errorf("no remote servers")
	}

	hcm := &healthCheckerManager{
		checkers:          make(map[string]*checker),
		remoteServers:     cfg.RemoteServers,
		remoteServerIndex: 0,
		sw:                sw,
		stopCh:            stopCh,
	}

	for _, remoteServer := range cfg.RemoteServers {
		c, err := newChecker(cfg, tp, remoteServer, hcm.setLastNodeLease, hcm.getLastNodeLease)
		if err != nil {
			return nil, err
		}
		hcm.checkers[remoteServer.String()] = c
		if c.check() {
			c.setHealthy(true)
		} else {
			c.setHealthy(false)
			klog.Warningf("cluster remote server %v is unhealthy.", remoteServer.String())
		}
	}
	return hcm, nil
}

func (hcm *healthCheckerManager) Run() {
	go hcm.healthzCheckLoop(hcm.stopCh)
	return
}

func (hcm *healthCheckerManager) healthzCheckLoop(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(heartbeatFrequency)
	defer intervalTicker.Stop()

	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in health check loop.")
			return
		case <-intervalTicker.C:
			hcm.sync()
		}
	}
}

func (hcm *healthCheckerManager) sync() {
	// Ensure that the node heartbeat can be reported when there is a healthy remote server.
	//try detect all remote server in a loop, if there is an remote server can update nodeLease, exit the loop.
	for i := 0; i < len(hcm.remoteServers); i++ {
		c := hcm.getChecker()
		if c.check() {
			break
		}
	}
}

func (hcm *healthCheckerManager) setLastNodeLease(lease *coordinationv1.Lease) error {
	if lease == nil {
		return nil
	}
	hcm.latestLease = lease

	accessor := meta.NewAccessor()
	accessor.SetKind(lease, coordinationv1.SchemeGroupVersion.WithKind("Lease").Kind)
	accessor.SetAPIVersion(lease, coordinationv1.SchemeGroupVersion.String())
	cacheLeaseKey := fmt.Sprintf(cacheLeaseKeyFormat, lease.Name)
	return hcm.sw.Update(cacheLeaseKey, lease)
}

func (hcm *healthCheckerManager) getLastNodeLease() *coordinationv1.Lease {
	return hcm.latestLease
}

func (hcm *healthCheckerManager) getChecker() *checker {
	checker := hcm.checkers[hcm.remoteServers[hcm.remoteServerIndex].String()]
	hcm.remoteServerIndex = (hcm.remoteServerIndex + 1) % len(hcm.remoteServers)
	return checker
}

// IsHealthy returns the healthy stats of specified server
func (hcm *healthCheckerManager) IsHealthy(server *url.URL) bool {
	if checker, ok := hcm.checkers[server.String()]; ok {
		return checker.isHealthy()
	}
	//if there is not checker for server, default unhealthy.
	return false
}

func newChecker(
	cfg *config.YurtHubConfiguration,
	tp transport.Interface,
	url *url.URL,
	setLastNodeLease setNodeLease,
	getLastNodeLease getNodeLease,
) (*checker, error) {
	restConf := &rest.Config{
		Host:      url.String(),
		Transport: tp.CurrentTransport(),
		Timeout:   time.Duration(cfg.HeartbeatTimeoutSeconds) * time.Second,
	}
	kubeClient, err := clientset.NewForConfig(restConf)
	if err != nil {
		return nil, err
	}

	nl := NewNodeLease(kubeClient, cfg.NodeName, defaultLeaseDurationSeconds, cfg.HeartbeatFailedRetry)
	c := &checker{
		nodeLease:        nl,
		lastTime:         time.Now(),
		clusterHealthy:   false,
		healthyThreshold: cfg.HeartbeatHealthyThreshold,
		healthyCnt:       0,
		remoteServer:     url,
		onFailureFunc:    tp.Close,
		setLastNodeLease: setLastNodeLease,
		getLastNodeLease: getLastNodeLease,
	}
	return c, nil
}

func (c *checker) check() bool {
	baseLease := c.getLastNodeLease()
	lease, err := c.nodeLease.Update(baseLease)
	if err == nil {
		if err := c.setLastNodeLease(lease); err != nil {
			klog.Errorf("set last node lease fail: %v", err)
		}
		c.healthyCnt++
		if !c.isHealthy() && c.healthyCnt >= c.healthyThreshold {
			c.setHealthy(true)
			now := time.Now()
			klog.Infof("cluster becomes healthy from %v, unhealthy status lasts %v, remote server: %v", now, now.Sub(c.lastTime), c.remoteServer.String())
			c.lastTime = now
			metrics.Metrics.ObserveServerHealthy(c.remoteServer.Host, 1)
		}
		return true
	}

	klog.Infof("failed to update lease: %v, remote server %s", err, c.remoteServer.String())
	c.healthyCnt = 0
	if c.isHealthy() {
		c.setHealthy(false)
		now := time.Now()
		klog.Infof("cluster becomes unhealthy from %v, healthy status lasts %v, remote server: %v", time.Now(), now.Sub(c.lastTime), c.remoteServer.String())
		c.lastTime = now
		if c.onFailureFunc != nil {
			c.onFailureFunc(c.remoteServer.Host)
		}
		metrics.Metrics.ObserveServerHealthy(c.remoteServer.Host, 0)
	}
	return false
}

func (c *checker) isHealthy() bool {
	c.RLock()
	defer c.RUnlock()
	return c.clusterHealthy
}

func (c *checker) setHealthy(healthy bool) {
	c.Lock()
	defer c.Unlock()
	c.clusterHealthy = healthy
}
