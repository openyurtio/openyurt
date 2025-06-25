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

package cloudapiserver

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
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
	probers           map[string]healthchecker.BackendProber
	latestLease       *coordinationv1.Lease
	sw                cachemanager.StorageWrapper
	remoteServerIndex int
	heartbeatInterval int
}

// NewCloudAPIServerHealthChecker returns a health checker for verifying cloud kube-apiserver status.
func NewCloudAPIServerHealthChecker(ctx context.Context, cfg *config.YurtHubConfiguration) (healthchecker.Interface, error) {
	hc := &cloudAPIServerHealthChecker{
		probers:           make(map[string]healthchecker.BackendProber),
		remoteServers:     cfg.RemoteServers,
		remoteServerIndex: 0,
		sw:                cfg.StorageWrapper,
		heartbeatInterval: cfg.HeartbeatIntervalSeconds,
	}

	for remoteServer, client := range cfg.TransportAndDirectClientManager.ListDirectClientset() {
		hc.probers[remoteServer] = newProber(client,
			remoteServer,
			cfg.NodeName,
			cfg.HeartbeatFailedRetry,
			cfg.HeartbeatHealthyThreshold,
			cfg.KubeletHealthGracePeriod,
			hc.setLastNodeLease,
			hc.getLastNodeLease)
	}
	if len(hc.probers) != 0 {
		go hc.run(ctx)
	}
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

func (hc *cloudAPIServerHealthChecker) PickOneHealthyBackend() *url.URL {
	for i := range hc.remoteServers {
		if hc.BackendIsHealthy(hc.remoteServers[i]) {
			return hc.remoteServers[i]
		}
	}

	return nil
}

// BackendHealthyStatus returns the healthy stats of specified server
func (hc *cloudAPIServerHealthChecker) BackendIsHealthy(server *url.URL) bool {
	if prober, ok := hc.probers[server.String()]; ok {
		return prober.IsHealthy()
	}
	// If there is no checker for server, default unhealthy.
	return false
}

func (hc *cloudAPIServerHealthChecker) UpdateBackends(servers []*url.URL) {
	// do nothing
}

func (hc *cloudAPIServerHealthChecker) run(ctx context.Context) {
	intervalTicker := time.NewTicker(time.Duration(hc.heartbeatInterval) * time.Second)
	defer intervalTicker.Stop()

	for {
		select {
		case <-ctx.Done():
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

func (hc *cloudAPIServerHealthChecker) getProber() healthchecker.BackendProber {
	prober := hc.probers[hc.remoteServers[hc.remoteServerIndex].String()]
	hc.remoteServerIndex = (hc.remoteServerIndex + 1) % len(hc.remoteServers)
	return prober
}
