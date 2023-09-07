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

package yurtcoordinator

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

const (
	InitHub int32 = iota // 0
	LeaderHub
	FollowerHub
	PendingHub
)

type HubElector struct {
	coordinatorClient           kubernetes.Interface
	coordinatorHealthChecker    healthchecker.HealthChecker
	cloudAPIServerHealthChecker healthchecker.MultipleBackendsHealthChecker
	electorStatus               chan int32
	le                          *leaderelection.LeaderElector
	inElecting                  bool
}

func NewHubElector(
	cfg *config.YurtHubConfiguration,
	coordinatorClient kubernetes.Interface,
	coordinatorHealthChecker healthchecker.HealthChecker,
	cloudAPIServerHealthyChecker healthchecker.MultipleBackendsHealthChecker,
	stopCh <-chan struct{}) (*HubElector, error) {
	he := &HubElector{
		coordinatorClient:           coordinatorClient,
		coordinatorHealthChecker:    coordinatorHealthChecker,
		cloudAPIServerHealthChecker: cloudAPIServerHealthyChecker,
		electorStatus:               make(chan int32, 1),
	}

	rl, err := resourcelock.New(cfg.LeaderElection.ResourceLock,
		cfg.LeaderElection.ResourceNamespace,
		cfg.LeaderElection.ResourceName,
		coordinatorClient.CoreV1(),
		coordinatorClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: cfg.NodeName})
	if err != nil {
		return nil, err
	}

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: cfg.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: cfg.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   cfg.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.Infof("yurthub of %s became leader", cfg.NodeName)
				he.electorStatus <- LeaderHub
			},
			OnStoppedLeading: func() {
				klog.Infof("yurthub of %s is no more a leader", cfg.NodeName)
				he.electorStatus <- FollowerHub
				he.inElecting = false
			},
		},
	})
	if err != nil {
		return nil, err
	}
	he.le = le
	he.electorStatus <- PendingHub

	return he, nil
}

func (he *HubElector) Run(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(5 * time.Second)
	defer intervalTicker.Stop()
	defer close(he.electorStatus)

	var ctx context.Context
	var cancel context.CancelFunc
	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in leader election loop.")

			if cancel != nil {
				cancel()
				he.inElecting = false
			}
			return
		case <-intervalTicker.C:
			if !he.coordinatorHealthChecker.IsHealthy() {
				if he.inElecting && cancel != nil {
					cancel()
					he.inElecting = false
					he.electorStatus <- PendingHub
				}
				break
			}

			if !he.cloudAPIServerHealthChecker.IsHealthy() {
				if he.inElecting && cancel != nil {
					cancel()
					he.inElecting = false
					he.electorStatus <- FollowerHub
				}
				break
			}

			if !he.inElecting {
				he.electorStatus <- FollowerHub
				ctx, cancel = context.WithCancel(context.TODO())
				go he.le.Run(ctx)
				he.inElecting = true
			}
		}
	}
}

func (he *HubElector) StatusChan() chan int32 {
	return he.electorStatus
}
