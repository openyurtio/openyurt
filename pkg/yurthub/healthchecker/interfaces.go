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
	"net/url"
	"time"
)

// HealthChecker is an interface for checking healthy status of one server
type HealthChecker interface {
	// RenewKubeletLeaseTime is used for notifying whether kubelet stopped or not,
	// when kubelet lease renew time is stopped to report, health checker will stop check
	// the healthy status of remote server and mark remote server as unhealthy.
	RenewKubeletLeaseTime()
	IsHealthy() bool
}

// MultipleBackendsHealthChecker is used for checking healthy status of multiple servers,
// like there are several kube-apiserver instances on the cloud for high availability.
type MultipleBackendsHealthChecker interface {
	HealthChecker
	BackendHealthyStatus(server *url.URL) bool
	PickHealthyServer() (*url.URL, error)
}

// BackendProber is used to send heartbeat to backend and verify backend
// is healthy or not
type BackendProber interface {
	// RenewKubeletLeaseTime is used for notifying whether kubelet stopped or not,
	// when kubelet lease renew time is stopped to report, health checker will stop check
	// the healthy status of remote server and mark remote server as unhealthy.
	RenewKubeletLeaseTime(time time.Time)
	// Probe send one heartbeat to backend and should be executed by caller in interval
	Probe(phase string) bool
	IsHealthy() bool
}
