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
	"time"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
)

const (
	Online  = iota // avalible
	Offline        // async
)

type MessageBusHealthChecker struct {
	messageBus        MessageBus
	status            chan int32
	heartbeatInterval int
	NodePool          string
	Namespace         string
	Host              string
	Name              string
}

func NewMessageBusHealthChecker(opts *options.YurtIoTDockOptions, msgBus MessageBus) (MessageBusHealthChecker, error) {

	mbhc := MessageBusHealthChecker{
		messageBus:        msgBus,
		status:            make(chan int32, 1),
		heartbeatInterval: opts.MessageBusOptions.HeartbeatInterval,
		NodePool:          opts.Nodepool,
		Namespace:         opts.Namespace,
		Host:              opts.MessageBusOptions.Host,
		Name:              opts.MessageBusOptions.Name,
	}

	return mbhc, nil
}

func (mbhc *MessageBusHealthChecker) Run(stopCh <-chan struct{}) {
	intervalTicker := time.NewTicker(time.Duration(mbhc.heartbeatInterval) * time.Second)
	defer intervalTicker.Stop()
	defer close(mbhc.status)

	for {
		select {
		case <-stopCh:
			klog.V(2).Info("exit normally in health check loop.")
			return
		case <-intervalTicker.C:
			if err := mbhc.messageBus.prepareMessageBus(); err != nil {
				klog.V(3).ErrorS(err, "failed to prepare message bus")
				mbhc.status <- Offline
				return
			}

			mbhc.status <- Online
		}
	}
}

func (mbhc *MessageBusHealthChecker) StatusChan() chan int32 {
	return mbhc.status
}
