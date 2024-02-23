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

package controllers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	kubetypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	util "github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

type Pulsimeter struct {
	// kubernetes client
	client.Client
	// which nodePool Plusimeter is deployed in
	NodePool string
	// which namespace Plusimeter is deployed in
	Namespace         string
	deviceSyncer      DeviceSyncer
	deviceProfileSync DeviceProfileSyncer
	deviceServiceSync DeviceServiceSyncer
	// messageBus
	messageBus util.MessageBus
	// messageBus health checker
	msgHealthChecker util.MessageBusHealthChecker
}

func NewPulsimeter(client client.Client, opts *options.YurtIoTDockOptions,
	edgexdock *edgexobj.EdgexDock) (Pulsimeter, error) {
	deviceSyncer, err := NewDeviceSyncer(client, opts, edgexdock)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to create device syncer")
		return Pulsimeter{}, err
	}
	deviceServiceSync, err := NewDeviceServiceSyncer(client, opts, edgexdock)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to create deviceservice syncer")
		return Pulsimeter{}, err
	}

	deviceProfileSync, err := NewDeviceProfileSyncer(client, opts, edgexdock)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to create deviceprofile syncer")
		return Pulsimeter{}, err
	}

	// init messagebus
	// TODO support other kind of messagebus
	messageBus, err := util.NewMessageBus(opts)

	if err != nil {
		klog.V(3).ErrorS(err, "could not init messagebus where yurt-iot-dock run")
		return Pulsimeter{}, err
	}

	msgHealthChecker, err := util.NewMessageBusHealthChecker(opts, messageBus)

	if err != nil {
		klog.V(3).ErrorS(err, "could not init messagebus checker where yurt-iot-dock run")
		return Pulsimeter{}, err
	}

	return Pulsimeter{
		Client:            client,
		NodePool:          opts.Nodepool,
		Namespace:         opts.Namespace,
		deviceSyncer:      deviceSyncer,
		deviceProfileSync: deviceProfileSync,
		deviceServiceSync: deviceServiceSync,
		msgHealthChecker:  msgHealthChecker,
		messageBus:        messageBus,
	}, nil

}

func (pm *Pulsimeter) NewPulsimeterRunnable() ctrlmgr.RunnableFunc {
	return func(ctx context.Context) error {
		go pm.msgHealthChecker.Run(ctx.Done())
		go pm.deviceSyncer.Run(ctx.Done())
		go pm.deviceProfileSync.Run(ctx.Done())
		go pm.deviceServiceSync.Run(ctx.Done())
		pm.Run(ctx.Done())
		return nil
	}
}

func (pm *Pulsimeter) Run(stop <-chan struct{}) {
	klog.V(1).Info("[Pulsimeter] Starting ...")
	go func() {
		klog.V(1).Info("start waiting for messages")
		for {
			select {
			case err := <-pm.messageBus.MessageErrors:
				klog.V(3).ErrorS(err, "fail to get the message")
			case msgEnvelope := <-pm.messageBus.Messages:
				var systemEvent dtos.SystemEvent
				err := json.Unmarshal(msgEnvelope.Payload, &systemEvent)
				if err != nil {
					klog.V(3).ErrorS(err, "fail to JSON decoding system event")
					continue
				}
				klog.V(1).Info("successful get the message")
				switch systemEvent.Type {
				case common.DeviceSystemEventType:
					if err := pm.dealWithDevice(systemEvent); err != nil {
						klog.V(3).ErrorS(err, "fail to deal with device systemEvent")
					}
				case common.DeviceServiceSystemEventType:
					if err := pm.dealWithDeviceService(systemEvent); err != nil {
						klog.V(3).ErrorS(err, "fail to deal with deviceservice systemEvent")
					}
				case common.DeviceProfileSystemEventType:
					if err := pm.dealWithDeviceProfile(systemEvent); err != nil {
						klog.V(3).ErrorS(err, "fail to deal with deviceprofile systemEvent")
					}
				}
			}
		}
	}()

	<-stop
	klog.V(1).Info("[Pulsimeter] Stopping ...")
}

// dealWithDevice focus on the change of device and apply these change to OpenYurt
func (pm *Pulsimeter) dealWithDevice(systemEvent dtos.SystemEvent) error {
	kubeDevice := iotv1alpha1.Device{}
	edgeDev, err := pm.deviceSyncer.deviceCli.Convert(context.TODO(), systemEvent, clients.GetOptions{Namespace: pm.Namespace})
	if err != nil {
		return err
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("deal with AddAction")
		newDevice := pm.deviceSyncer.completeCreateContent(edgeDev)
		if err := pm.Client.Create(context.TODO(), newDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to create device on OpenYurt", "DeviceName", strings.ToLower(newDevice.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("deal with UpdateAction")
		name := strings.Join([]string{pm.NodePool, edgeDev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to get device on OpenYurt", "DeviceName", strings.ToLower(edgeDev.Name))
			return err
		}

		pm.deviceSyncer.completeUpdateContent(&kubeDevice, edgeDev)
		if err := pm.Client.Update(context.TODO(), &kubeDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to update device on OpenYurt", "DeviceName", strings.ToLower(edgeDev.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("deal with DeleteAction")
		name := strings.Join([]string{pm.NodePool, edgeDev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to get device on OpenYurt", "DeviceName", strings.ToLower(edgeDev.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to delete device on OpenYurt", "DeviceName", strings.ToLower(edgeDev.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDevice, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceName", strings.ToLower(edgeDev.Name))
			return err
		}
	}

	return nil
}

// dealWithDeviceService focus on the change of deviceservice and apply these change to OpenYurt
func (pm *Pulsimeter) dealWithDeviceService(systemEvent dtos.SystemEvent) error {
	kubeDs := iotv1alpha1.DeviceService{}
	edgeDs, err := pm.deviceServiceSync.deviceServiceCli.Convert(context.TODO(), systemEvent, clients.GetOptions{Namespace: pm.Namespace})
	if err != nil {
		return err
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("[Pulsimeter]: deal with the AddAction of DeviceService")
		newDs := pm.deviceServiceSync.completeCreateContent(edgeDs)
		if err := pm.Client.Create(context.TODO(), newDs); err != nil {
			klog.V(3).ErrorS(err, "fail to create deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("[Pulsimeter]: deal with UpdateAction of DeviceService")
		name := strings.Join([]string{pm.NodePool, edgeDs.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDs); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}

		pm.deviceServiceSync.completeUpdateContent(&kubeDs, edgeDs)
		if err := pm.Client.Update(context.TODO(), &kubeDs); err != nil {
			klog.V(3).ErrorS(err, "fail to update deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("[Pulsimeter]: deal with DeleteAction of DeviceService")
		name := strings.Join([]string{pm.NodePool, edgeDs.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDs); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDs); err != nil {
			klog.V(3).ErrorS(err, "fail to delete deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDs, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceServiceName", strings.ToLower(edgeDs.Name))
			return err
		}
	}

	return nil
}

// dealWithDeviceProfile focus on the change of deviceprofile and apply these change to OpenYurt
func (pm *Pulsimeter) dealWithDeviceProfile(systemEvent dtos.SystemEvent) error {
	kubeDprofile := iotv1alpha1.DeviceProfile{}
	edgeDprofile, err := pm.deviceProfileSync.edgeClient.Convert(context.TODO(), systemEvent, clients.GetOptions{Namespace: pm.Namespace})
	if err != nil {
		return err
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("deal with AddAction")
		newDp := pm.deviceProfileSync.completeCreateContent(edgeDprofile)
		if err := pm.Client.Create(context.TODO(), newDp); err != nil {
			klog.V(3).ErrorS(err, "fail to create deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("deal with UpdateAction")
		name := strings.Join([]string{pm.NodePool, edgeDprofile.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDprofile); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}

		pm.deviceProfileSync.completeUpdateContent(&kubeDprofile, edgeDprofile)
		if err := pm.Client.Update(context.TODO(), &kubeDprofile); err != nil {
			klog.V(3).ErrorS(err, "fail to update deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("deal with DeleteAction")
		name := strings.Join([]string{pm.NodePool, edgeDprofile.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDprofile); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDprofile); err != nil {
			klog.V(3).ErrorS(err, "fail to delete deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDprofile, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceProfileName", strings.ToLower(edgeDprofile.Name))
			return err
		}
	}

	return nil
}
