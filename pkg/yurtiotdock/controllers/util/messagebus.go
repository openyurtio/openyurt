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
	"github.com/avast/retry-go"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/common"
	"github.com/edgexfoundry/go-mod-messaging/v3/messaging"
	"github.com/edgexfoundry/go-mod-messaging/v3/pkg/types"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
)

type MessageBus struct {
	msgBusClient  messaging.MessageClient
	Messages      chan types.MessageEnvelope
	MessageErrors chan error
}

func NewMessageBus(opts *options.YurtIoTDockOptions) (MessageBus, error) {
	msgClient, err := messaging.NewMessageClient(
		types.MessageBusConfig{
			Broker: types.HostInfo{
				Host:     opts.MessageBusOptions.Host,
				Port:     opts.MessageBusOptions.Port,
				Protocol: opts.MessageBusOptions.Protocol,
			},
			Type: opts.MessageBusOptions.Type,
		})

	if err != nil {
		klog.V(3).ErrorS(err, "failed to create messaging client")
		return MessageBus{}, err
	}

	return MessageBus{
		msgBusClient:  msgClient,
		Messages:      make(chan types.MessageEnvelope, 1),
		MessageErrors: make(chan error, 1),
	}, nil
}

func (mb *MessageBus) prepareMessageBus() error {
	// topic is like edgex/system-events/core-metadata/#
	topics := []types.TopicChannel{
		{
			Topic:    common.BuildTopic(common.DefaultBaseTopic, common.SystemEventPublishTopic, common.CoreMetaDataServiceKey, "#"),
			Messages: mb.Messages,
		},
	}

	if err := mb.msgBusClient.Connect(); err != nil {
		klog.V(3).ErrorS(err, "failed to connect to message bus")
		return err
	}

	// make sure the messagebus is builded
	err := retry.Do(
		func() error {
			err := mb.msgBusClient.Subscribe(topics, mb.MessageErrors)
			if err != nil {
				klog.V(3).ErrorS(err, "fail to subscribe the topic")
			}
			return err
		},
	)
	return err
}
