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

package poolcoordinator

import (
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

type Coordinator struct {
	coordinatorHealthChecker healthchecker.HealthChecker
	hubElector               *HubElector
	informerStarted          bool
}

func NewCoordinator(coordinatorHealthChecker healthchecker.HealthChecker, elector *HubElector, stopCh <-chan struct{}) *Coordinator {
	return &Coordinator{
		coordinatorHealthChecker: coordinatorHealthChecker,
		hubElector:               elector,
	}
}

func (coordinator *Coordinator) Run(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			klog.Infof("exit normally in coordinator loop.")
			if coordinator.informerStarted {
				// stop shared informer
				//
				coordinator.informerStarted = false
			}
			return
		case electorStatus, ok := <-coordinator.hubElector.StatusChan():
			if !ok {
				return
			}

			if electorStatus != PendingHub && !coordinator.cacheIsUploaded() {
				// upload local cache, and make sure yurthub pod is the last resource uploaded
			}

			if electorStatus == LeaderHub {
				if !coordinator.informerStarted {
					coordinator.informerStarted = true
					// start shared informer for pool-scope data
					// make sure

					// start shared informer for lease delegating
					//
				}
				break
			}

			if electorStatus == FollowerHub {
				if coordinator.informerStarted {
					// stop shared informer
					//
					coordinator.informerStarted = false
				}
			}
		}
	}
}

func (coordinator *Coordinator) cacheIsUploaded() bool {
	// check yurthub pod is uploaded
	return true
}

func (coordinator *Coordinator) IsReady() bool {

	return true
}
