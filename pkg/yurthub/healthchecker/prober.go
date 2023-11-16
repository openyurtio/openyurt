/*
Copyright 2022 The OpenYurt Authors.

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
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
)

const (
	ProbePhaseInit   = "init"
	ProbePhaseNormal = "normal"
)

type prober struct {
	sync.RWMutex
	remoteServer           string
	clusterHealthy         bool
	healthyThreshold       int
	healthyCnt             int
	lastTime               time.Time
	lastRenewTime          time.Time
	healthCheckGracePeriod time.Duration
	nodeLease              NodeLease
	getLastNodeLease       getNodeLease
	setLastNodeLease       setNodeLease
}

func newProber(
	kubeClient kubernetes.Interface,
	remoteServer string,
	nodeName string,
	heartbeatFailedRetry int,
	heartbeatHealthyThreshold int,
	healthCheckGracePeriod time.Duration,
	setLastNodeLease setNodeLease,
	getLastNodeLease getNodeLease,
) BackendProber {
	nl := NewNodeLease(kubeClient, nodeName, int32(healthCheckGracePeriod.Seconds()), heartbeatFailedRetry)
	p := &prober{
		nodeLease:              nl,
		lastTime:               time.Now(),
		clusterHealthy:         false,
		healthyThreshold:       heartbeatHealthyThreshold,
		healthCheckGracePeriod: healthCheckGracePeriod,
		healthyCnt:             0,
		remoteServer:           remoteServer,
		setLastNodeLease:       setLastNodeLease,
		getLastNodeLease:       getLastNodeLease,
	}

	p.Probe(ProbePhaseInit)
	return p
}

func (p *prober) RenewKubeletLeaseTime(renewTime time.Time) {
	p.Lock()
	defer p.Unlock()
	p.lastRenewTime = renewTime
}

func (p *prober) Probe(phase string) bool {
	if p.kubeletStopped() {
		p.markAsUnhealthy(phase)
		return false
	}

	baseLease := p.getLastNodeLease()
	lease, err := p.nodeLease.Update(baseLease)
	if err == nil {
		if err := p.setLastNodeLease(lease); err != nil {
			klog.Errorf("could not store last node lease: %v", err)
		}
		p.markAsHealthy(phase)
		return true
	}

	klog.Errorf("could not probe: %v, remote server %s", err, p.ServerName())
	p.markAsUnhealthy(phase)
	return false
}

func (p *prober) IsHealthy() bool {
	p.RLock()
	defer p.RUnlock()
	return p.clusterHealthy
}

func (p *prober) ServerName() string {
	return p.remoteServer
}

func (p *prober) kubeletStopped() bool {
	p.Lock()
	defer p.Unlock()
	stopped := false
	if !p.lastRenewTime.IsZero() && p.healthCheckGracePeriod > 0 {
		stopped = time.Now().After(p.lastRenewTime.Add(p.healthCheckGracePeriod))
	}

	return stopped
}

func (p *prober) setHealthy(healthy bool) {
	p.Lock()
	defer p.Unlock()
	p.clusterHealthy = healthy
}

func (p *prober) markAsHealthy(phase string) {
	p.healthyCnt++
	if phase == ProbePhaseInit {
		klog.Infof("healthy status of remote server %s in %s phase is healthy", p.ServerName(), phase)
		p.setHealthy(true)
		return
	}

	if !p.IsHealthy() && p.healthyCnt >= p.healthyThreshold {
		p.setHealthy(true)
		now := time.Now()
		klog.Infof("remote server %s becomes healthy from %v, unhealthy status lasts %v", p.ServerName(), now, now.Sub(p.lastTime))
		p.lastTime = now
		metrics.Metrics.ObserveServerHealthy(p.ServerName(), 1)
	}
}

func (p *prober) markAsUnhealthy(phase string) {
	p.healthyCnt = 0
	if phase == ProbePhaseInit {
		klog.Infof("healthy status of remote server %s in %s phase is unhealthy", p.ServerName(), phase)
		p.setHealthy(false)
		return
	}

	if p.IsHealthy() {
		p.setHealthy(false)
		now := time.Now()
		klog.Infof("remote server %s becomes unhealthy from %v, healthy status lasts %v", p.ServerName(), time.Now(), now.Sub(p.lastTime))
		p.lastTime = now
		metrics.Metrics.ObserveServerHealthy(p.ServerName(), 0)
	}
}
