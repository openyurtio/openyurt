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
	"strconv"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

const (
	DelegateHeartBeat = "openyurt.io/delegate-heartbeat"
)

type setNodeLease func(*coordinationv1.Lease) error
type getNodeLease func() *coordinationv1.Lease

type cloudAPIServerHealthChecker struct {
	sync.RWMutex
	remoteServers     []*url.URL
	probers           map[string]BackendProber
	latestLease       *coordinationv1.Lease
	sw                cachemanager.StorageWrapper
	remoteServerIndex int
	heartbeatInterval int
}

type coordinatorHealthChecker struct {
	sync.RWMutex
	cloudServerHealthChecker HealthChecker
	coordinatorProber        BackendProber
	latestLease              *coordinationv1.Lease
	heartbeatInterval        int
}

// NewCoordinatorHealthChecker returns a health checker for verifying yurt coordinator status.
func NewCoordinatorHealthChecker(cfg *config.YurtHubConfiguration, checkerClient kubernetes.Interface, cloudServerHealthChecker HealthChecker, stopCh <-chan struct{}) (HealthChecker, error) {
	chc := &coordinatorHealthChecker{
		cloudServerHealthChecker: cloudServerHealthChecker,
		heartbeatInterval:        cfg.HeartbeatIntervalSeconds,
	}
	chc.coordinatorProber = newProber(checkerClient,
		cfg.CoordinatorServerURL.String(),
		cfg.NodeName,
		cfg.HeartbeatFailedRetry,
		cfg.HeartbeatHealthyThreshold,
		cfg.KubeletHealthGracePeriod,
		chc.setLastNodeLease,
		chc.getLastNodeLease)
	go chc.run(stopCh)

	return chc, nil
}

func (chc *coordinatorHealthChecker) IsHealthy() bool {
	return chc.coordinatorProber.IsHealthy()
}

func (chc *coordinatorHealthChecker) RenewKubeletLeaseTime() {
	chc.coordinatorProber.RenewKubeletLeaseTime(time.Now())
}

func (chc *coordinatorHealthChecker) run(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(time.Duration(chc.heartbeatInterval) * time.Second)
	defer intervalTicker.Stop()

	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in health check loop.")
			return
		case <-intervalTicker.C:
			chc.coordinatorProber.Probe(ProbePhaseNormal)
		}
	}
}

func (chc *coordinatorHealthChecker) setLastNodeLease(lease *coordinationv1.Lease) error {
	if lease == nil {
		return nil
	}
	chc.latestLease = lease
	return nil
}

func (chc *coordinatorHealthChecker) getLastNodeLease() *coordinationv1.Lease {
	if chc.latestLease != nil {
		if !chc.cloudServerHealthChecker.IsHealthy() {
			if chc.latestLease.Annotations == nil {
				chc.latestLease.Annotations = make(map[string]string)
			}
			chc.latestLease.Annotations[DelegateHeartBeat] = "true"
		} else {
			delete(chc.latestLease.Annotations, DelegateHeartBeat)
		}
	}

	return chc.latestLease
}

// NewCloudAPIServerHealthChecker returns a health checker for verifying cloud kube-apiserver status.
func NewCloudAPIServerHealthChecker(cfg *config.YurtHubConfiguration, healthCheckerClients map[string]kubernetes.Interface, stopCh <-chan struct{}) (MultipleBackendsHealthChecker, error) {
	if len(healthCheckerClients) == 0 {
		return nil, fmt.Errorf("no remote servers")
	}

	hc := &cloudAPIServerHealthChecker{
		probers:           make(map[string]BackendProber),
		remoteServers:     cfg.RemoteServers,
		remoteServerIndex: 0,
		sw:                cfg.StorageWrapper,
		heartbeatInterval: cfg.HeartbeatIntervalSeconds,
	}

	for remoteServer, client := range healthCheckerClients {
		hc.probers[remoteServer] = newProber(client,
			remoteServer,
			cfg.NodeName,
			cfg.HeartbeatFailedRetry,
			cfg.HeartbeatHealthyThreshold,
			cfg.KubeletHealthGracePeriod,
			hc.setLastNodeLease,
			hc.getLastNodeLease)
	}
	go hc.run(stopCh)
	return hc, nil
}

func (hc *cloudAPIServerHealthChecker) RenewKubeletLeaseTime() {
	currentTime := time.Now()
	for _, prober := range hc.probers {
		prober.RenewKubeletLeaseTime(currentTime)
	}
}

func (hc *cloudAPIServerHealthChecker) IsHealthy() bool {
	for _, prober := range hc.probers {
		if prober.IsHealthy() {
			return true
		}
	}

	return false
}

func (hc *cloudAPIServerHealthChecker) PickHealthyServer() (*url.URL, error) {
	for server, prober := range hc.probers {
		if prober.IsHealthy() {
			return url.Parse(server)
		}
	}

	return nil, nil
}

// BackendHealthyStatus returns the healthy stats of specified server
func (hc *cloudAPIServerHealthChecker) BackendHealthyStatus(server *url.URL) bool {
	if prober, ok := hc.probers[server.String()]; ok {
		return prober.IsHealthy()
	}
	// If there is no checker for server, default unhealthy.
	return false
}

func (hc *cloudAPIServerHealthChecker) run(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(time.Duration(hc.heartbeatInterval) * time.Second)
	defer intervalTicker.Stop()

	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in health check loop.")
			return
		case <-intervalTicker.C:
			// Ensure that the node heartbeat can be reported when there is a healthy remote server.
			// Try to detect all remote server in a loop, if there is a remote server can update nodeLease, exit the loop.
			for i := 0; i < len(hc.remoteServers); i++ {
				p := hc.getProber()
				if p.Probe(ProbePhaseNormal) {
					break
				}
			}
		}
	}
}

func (hc *cloudAPIServerHealthChecker) setLastNodeLease(lease *coordinationv1.Lease) error {
	if lease == nil {
		return nil
	}
	hc.latestLease = lease

	accessor := meta.NewAccessor()
	accessor.SetKind(lease, coordinationv1.SchemeGroupVersion.WithKind("Lease").Kind)
	accessor.SetAPIVersion(lease, coordinationv1.SchemeGroupVersion.String())
	leaseKey, err := hc.sw.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Namespace: lease.Namespace,
		Name:      lease.Name,
		Resources: "leases",
		Group:     "coordination.k8s.io",
		Version:   "v1",
	})
	if err != nil {
		return fmt.Errorf("could not get key for lease %s/%s, %v", lease.Namespace, lease.Name, err)
	}
	rv, err := strconv.ParseUint(lease.ResourceVersion, 10, 64)
	if err != nil {
		return fmt.Errorf("could not convert rv string %s of lease %s/%s, %v", lease.ResourceVersion, lease.Namespace, lease.Name, err)
	}
	_, err = hc.sw.Update(leaseKey, lease, rv)
	if err == storage.ErrStorageNotFound {
		klog.Infof("find no lease of %s in storage, init a new one", leaseKey.Key())
		if err := hc.sw.Create(leaseKey, lease); err != nil {
			return fmt.Errorf("could not create the lease %s, %v", leaseKey.Key(), err)
		}
	} else if err != nil {
		return fmt.Errorf("could not update lease %s/%s, %v", lease.Namespace, lease.Name, err)
	}
	return nil
}

func (hc *cloudAPIServerHealthChecker) getLastNodeLease() *coordinationv1.Lease {
	if hc.latestLease != nil {
		delete(hc.latestLease.Annotations, DelegateHeartBeat)
	}
	return hc.latestLease
}

func (hc *cloudAPIServerHealthChecker) getProber() BackendProber {
	prober := hc.probers[hc.remoteServers[hc.remoteServerIndex].String()]
	hc.remoteServerIndex = (hc.remoteServerIndex + 1) % len(hc.remoteServers)
	return prober
}
