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
	"github.com/edgexfoundry/go-mod-messaging/v3/messaging"
	"github.com/edgexfoundry/go-mod-messaging/v3/pkg/types"
	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/avast/retry-go"
	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	devcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	iotcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	utilv3 "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry/v3"
	util "github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubetypes "k8s.io/apimachinery/pkg/types"
)

type Pulsimeter struct {
	// kubernetes client
	client.Client
	// which nodePool Plusimeter is deployed in
	NodePool string
	// which namespace Plusimeter is deployed in
	Namespace string
	// messageBus
	MessgaeBus messaging.MessageClient
	listHelper ListHelper
}

type ListHelper struct {
	deviceServiceList DeviceServiceList
	deviceProfileList DeviceProfileList
}

type DeviceServiceList struct {
	// Kubernetes client
	client.Client
	deviceServiceCli iotcli.DeviceServiceInterface
	NodePool         string
	Namespace        string
}

type DeviceProfileList struct {
	// edge platform client
	edgeClient devcli.DeviceProfileInterface
	// Kubernetes client
	client.Client
	NodePool  string
	Namespace string
}

func NewPulsimeter(client client.Client, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) (Pulsimeter, error) {

	// init messagebus
	// TODO support other kind of messagebus
	messagebus, err := messaging.NewMessageClient(types.MessageBusConfig{
		Broker: types.HostInfo{
			Host:     opts.RedisAddr,
			Port:     int(opts.RedisPort),
			Protocol: "redis",
		},
		Type: "redis"})

	if err != nil {
		return Pulsimeter{}, err
	}
	deviceserviceclient, err := edgexdock.CreateDeviceServiceClient()
	if err != nil {
		return Pulsimeter{}, err
	}
	deviceServiceList := DeviceServiceList{
		Client:           client,
		NodePool:         opts.Nodepool,
		Namespace:        opts.Namespace,
		deviceServiceCli: deviceserviceclient,
	}
	edgeclient, err := edgexdock.CreateDeviceProfileClient()
	if err != nil {
		return Pulsimeter{}, err
	}
	deviceProfileList := DeviceProfileList{
		edgeClient: edgeclient,
		Client:     client,
		NodePool:   opts.Nodepool,
		Namespace:  opts.Namespace,
	}

	helper := ListHelper{
		deviceServiceList: deviceServiceList,
		deviceProfileList: deviceProfileList,
	}

	return Pulsimeter{
		Client:     client,
		NodePool:   opts.Nodepool,
		Namespace:  opts.Namespace,
		MessgaeBus: messagebus,
		listHelper: helper,
	}, nil

}

func (pm *Pulsimeter) NewPulsimeterRunnable() ctrlmgr.RunnableFunc {
	return func(ctx context.Context) error {
		pm.Run(ctx.Done())
		return nil
	}
}

func (pm *Pulsimeter) Run(stop <-chan struct{}) {
	klog.V(1).Info("[Pulsimeter] Starting ...")

	// list all the metadata information
	pm.listHelper.list()
	// prepare the topic and channel for subscribe
	// topic is like edgex/system-events/core-metadata/#
	topic := common.BuildTopic(common.DefaultBaseTopic, common.SystemEventPublishTopic, common.CoreMetaDataServiceKey, "#")
	messages := make(chan types.MessageEnvelope, 1)
	messageErrors := make(chan error, 1)
	topics := []types.TopicChannel{
		{
			Topic:    topic,
			Messages: messages,
		},
	}
	// make sure the messagebus is builded
	err := retry.Do(
		func() error {
			err := pm.MessgaeBus.Subscribe(topics, messageErrors)
			if err != nil {
				klog.V(3).ErrorS(err, "fail to subscribe the topic")
			}
			return err
		},
	)
	if err != nil {
		klog.V(3).ErrorS(err, "retry subscribing the topic error")
	}

	klog.V(1).Info("start waiting for messages")
	go func() {
		for {
			select {
			case err = <-messageErrors:
				klog.V(3).ErrorS(err, "fail to get the message")
			case msgEnvelope := <-messages:
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
	dto := dtos.Device{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode device system event details")
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("deal with AddAction")
		dev := utilv3.ToKubeDevice(dto, pm.Namespace)
		newDevice := pm.completeDeviceCreateContent(&dev)
		if err := pm.Client.Create(context.TODO(), newDevice); err != nil {
			klog.V(3).ErrorS(err, "fail to create device on OpenYurt", "DeviceName", strings.ToLower(newDevice.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("deal with UpdateAction")
		dev := utilv3.ToKubeDevice(dto, pm.Namespace)

		kubeDev := iotv1alpha1.Device{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDev); err != nil {
			klog.V(3).ErrorS(err, "fail to get device on OpenYurt", "DeviceName", strings.ToLower(dev.Name))
			return err
		}

		pm.updateDevice(&kubeDev, &dev)
		if err := pm.Client.Update(context.TODO(), &kubeDev); err != nil {
			klog.V(3).ErrorS(err, "fail to update device on OpenYurt", "DeviceName", strings.ToLower(dev.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("deal with DeleteAction")
		dev := utilv3.ToKubeDevice(dto, pm.Namespace)

		kubeDev := iotv1alpha1.Device{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDev); err != nil {
			klog.V(3).ErrorS(err, "fail to get device on OpenYurt", "DeviceName", strings.ToLower(dev.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDev); err != nil {
			klog.V(3).ErrorS(err, "fail to delete device on OpenYurt", "DeviceName", strings.ToLower(dev.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDev, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceName", strings.ToLower(dev.Name))
			return err
		}
	}

	return nil
}

// completeDeviceCreateContent completes the content of the device which will be created on OpenYurt
func (pm *Pulsimeter) completeDeviceCreateContent(edgeDevice *iotv1alpha1.Device) *iotv1alpha1.Device {
	createDevice := edgeDevice.DeepCopy()
	createDevice.Spec.NodePool = pm.NodePool
	createDevice.Name = strings.Join([]string{pm.NodePool, createDevice.Name}, "-")
	createDevice.Namespace = pm.Namespace
	createDevice.Spec.Managed = false

	return createDevice
}

// updateDevice completes the content of the device which will be updated on OpenYurt
func (pm *Pulsimeter) updateDevice(kubeDev *iotv1alpha1.Device, newDev *iotv1alpha1.Device) {
	updateDevice := pm.completeDeviceCreateContent(newDev)
	kubeDev.Spec = updateDevice.Spec
	kubeDev.Status.OperatingState = updateDevice.Spec.OperatingState
	kubeDev.Status.AdminState = updateDevice.Spec.AdminState
}

// dealWithDeviceService focus on the change of deviceservice and apply these change to OpenYurt
func (pm *Pulsimeter) dealWithDeviceService(systemEvent dtos.SystemEvent) error {
	dto := dtos.DeviceService{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode deviceservice systemEvent details")
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("deal with AddAction")
		dev := utilv3.ToKubeDeviceService(dto, pm.Namespace)
		newDS := pm.completeDeviceServiceCreateContent(&dev)
		if err := pm.Client.Create(context.TODO(), newDS); err != nil {
			klog.V(3).ErrorS(err, "fail to create deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("deal with UpdateAction")
		dev := utilv3.ToKubeDeviceService(dto, pm.Namespace)

		kubeDS := iotv1alpha1.DeviceService{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDS); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}

		pm.updateDeviceService(&kubeDS, &dev)
		if err := pm.Client.Update(context.TODO(), &kubeDS); err != nil {
			klog.V(3).ErrorS(err, "fail to update deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("deal with DeleteAction")
		dev := utilv3.ToKubeDeviceService(dto, pm.Namespace)

		kubeDS := iotv1alpha1.DeviceService{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDS); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDS); err != nil {
			klog.V(3).ErrorS(err, "fail to delete deviceservice on OpenYurt", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDS, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceServiceName", strings.ToLower(dev.Name))
			return err
		}
	}

	return nil
}

// completeDeviceServiceCreateContent completes the content of the device which will be created on OpenYurt
func (pm *Pulsimeter) completeDeviceServiceCreateContent(edgeDS *iotv1alpha1.DeviceService) *iotv1alpha1.DeviceService {
	createDS := edgeDS.DeepCopy()
	createDS.Spec.NodePool = pm.NodePool
	createDS.Namespace = pm.Namespace
	createDS.Name = strings.Join([]string{pm.NodePool, edgeDS.Name}, "-")
	createDS.Spec.Managed = false
	return createDS
}

// updateDeviceService completes the content of the device which will be updated on OpenYurt
func (pm *Pulsimeter) updateDeviceService(kubeDS *iotv1alpha1.DeviceService, newDS *iotv1alpha1.DeviceService) {
	updateDS := pm.completeDeviceServiceCreateContent(newDS)
	kubeDS.Spec = updateDS.Spec
	kubeDS.Status.AdminState = updateDS.Spec.AdminState
}

// dealWithDeviceProfile focus on the change of deviceprofile and apply these change to OpenYurt
func (pm *Pulsimeter) dealWithDeviceProfile(systemEvent dtos.SystemEvent) error {
	dto := dtos.DeviceProfile{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode deviceprofile systemEvent details")
	}

	switch systemEvent.Action {
	case common.SystemEventActionAdd:
		klog.V(2).Info("deal with AddAction")
		dev := utilv3.ToKubeDeviceProfile(dto, pm.Namespace)
		newDP := pm.completeDeviceProfileCreateContent(&dev)
		if err := pm.Client.Create(context.TODO(), newDP); err != nil {
			klog.V(3).ErrorS(err, "fail to create deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}

	case common.SystemEventActionUpdate:
		klog.V(2).Info("deal with UpdateAction")
		dev := utilv3.ToKubeDeviceProfile(dto, pm.Namespace)

		kubeDP := iotv1alpha1.DeviceProfile{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDP); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}

		pm.updateDeviceProfile(&kubeDP, &dev)
		if err := pm.Client.Update(context.TODO(), &kubeDP); err != nil {
			klog.V(3).ErrorS(err, "fail to update deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}

	case common.SystemEventActionDelete:
		klog.V(2).Info("deal with DeleteAction")
		dev := utilv3.ToKubeDeviceProfile(dto, pm.Namespace)

		kubeDP := iotv1alpha1.DeviceProfile{}
		name := strings.Join([]string{pm.NodePool, dev.Name}, "-")
		objectKey := client.ObjectKey{Namespace: pm.Namespace, Name: name}
		if err := pm.Client.Get(context.TODO(), objectKey, &kubeDP); err != nil {
			klog.V(3).ErrorS(err, "fail to get deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}
		if err := pm.Client.Delete(context.TODO(), &kubeDP); err != nil {
			klog.V(3).ErrorS(err, "fail to delete deviceprofile on OpenYurt", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})

		if err = pm.Client.Patch(context.TODO(), &kubeDP, client.RawPatch(kubetypes.MergePatchType, patchData)); err != nil {
			klog.V(3).ErrorS(err, "fail to remove the finalizer", "DeviceProfileName", strings.ToLower(dev.Name))
			return err
		}
	}

	return nil
}

// completeCreateContent completes the content of the deviceProfile which will be created on OpenYurt
func (pm *Pulsimeter) completeDeviceProfileCreateContent(edgeDps *iotv1alpha1.DeviceProfile) *iotv1alpha1.DeviceProfile {
	createDeviceProfile := edgeDps.DeepCopy()
	createDeviceProfile.Namespace = pm.Namespace
	createDeviceProfile.Name = strings.Join([]string{pm.NodePool, createDeviceProfile.Name}, "-")
	createDeviceProfile.Spec.NodePool = pm.NodePool
	return createDeviceProfile
}

// updateDeviceProfile completes the content of the deviceProfile which will be updated on OpenYurt
func (pm *Pulsimeter) updateDeviceProfile(kubeDP *iotv1alpha1.DeviceProfile, newDP *iotv1alpha1.DeviceProfile) {
	updateDS := pm.completeDeviceProfileCreateContent(newDP)
	kubeDP.Spec = updateDS.Spec
}

// DeviceServiceList
func (ds *DeviceServiceList) ServiceList() {

	// 1. get deviceServices on edge platform and OpenYurt
	edgeDeviceServices, kubeDeviceServices, err := ds.getAllDeviceServices()
	if err != nil {
		klog.V(3).ErrorS(err, "fail to list the deviceServices")
	}

	// 2. find the deviceServices that need to be synchronized
	redundantEdgeDeviceServices, redundantKubeDeviceServices, syncedDeviceServices :=
		ds.findDiffDeviceServices(edgeDeviceServices, kubeDeviceServices)
	klog.V(2).Infof("[DeviceService] The number of objects waiting for synchronization { %s:%d, %s:%d, %s:%d }",
		"Edge deviceServices should be added to OpenYurt", len(redundantEdgeDeviceServices),
		"OpenYurt deviceServices that should be deleted", len(redundantKubeDeviceServices),
		"DeviceServices that should be synchronized", len(syncedDeviceServices))

	// 3. create deviceServices on OpenYurt which are exists in edge platform but not in OpenYurt
	if err := ds.syncEdgeToKube(redundantEdgeDeviceServices); err != nil {
		klog.V(3).ErrorS(err, "fail to create deviceServices on OpenYurt")
	}

	// 4. delete redundant deviceServices on OpenYurt
	if err := ds.deleteDeviceServices(redundantKubeDeviceServices); err != nil {
		klog.V(3).ErrorS(err, "fail to delete redundant deviceServices on OpenYurt")
	}

	// 5. update deviceService status on OpenYurt
	if err := ds.updateDeviceServices(syncedDeviceServices); err != nil {
		klog.V(3).ErrorS(err, "fail to update deviceServices")
	}
	klog.V(2).Info("[DeviceService] synchronization is complete")

}

// Get the existing DeviceService on the Edge platform, as well as OpenYurt existing DeviceService
// edgeDeviceServices：map[actualName]DeviceService
// kubeDeviceServices：map[actualName]DeviceService
func (ds *DeviceServiceList) getAllDeviceServices() (
	map[string]iotv1alpha1.DeviceService, map[string]iotv1alpha1.DeviceService, error) {

	edgeDeviceServices := map[string]iotv1alpha1.DeviceService{}
	kubeDeviceServices := map[string]iotv1alpha1.DeviceService{}

	// 1. list deviceServices on edge platform
	eDevSs, err := ds.deviceServiceCli.List(context.TODO(), iotcli.ListOptions{Namespace: ds.Namespace})
	if err != nil {
		klog.V(4).ErrorS(err, "fail to list the deviceServices object on the edge platform")
		return edgeDeviceServices, kubeDeviceServices, err
	}
	// 2. list deviceServices on OpenYurt (filter objects belonging to edgeServer)
	var kDevSs iotv1alpha1.DeviceServiceList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: ds.NodePool}
	if err = ds.List(context.TODO(), &kDevSs, listOptions, client.InNamespace(ds.Namespace)); err != nil {
		klog.V(4).ErrorS(err, "fail to list the deviceServices object on the Kubernetes")
		return edgeDeviceServices, kubeDeviceServices, err
	}
	for i := range eDevSs {
		deviceServicesName := util.GetEdgeDeviceServiceName(&eDevSs[i], EdgeXObjectName)
		edgeDeviceServices[deviceServicesName] = eDevSs[i]
	}

	for i := range kDevSs.Items {
		deviceServicesName := util.GetEdgeDeviceServiceName(&kDevSs.Items[i], EdgeXObjectName)
		kubeDeviceServices[deviceServicesName] = kDevSs.Items[i]
	}
	return edgeDeviceServices, kubeDeviceServices, nil
}

// Get the list of deviceServices that need to be added, deleted and updated
func (ds *DeviceServiceList) findDiffDeviceServices(
	edgeDeviceService map[string]iotv1alpha1.DeviceService, kubeDeviceService map[string]iotv1alpha1.DeviceService) (
	redundantEdgeDeviceServices map[string]*iotv1alpha1.DeviceService, redundantKubeDeviceServices map[string]*iotv1alpha1.DeviceService, syncedDeviceServices map[string]*iotv1alpha1.DeviceService) {

	redundantEdgeDeviceServices = map[string]*iotv1alpha1.DeviceService{}
	redundantKubeDeviceServices = map[string]*iotv1alpha1.DeviceService{}
	syncedDeviceServices = map[string]*iotv1alpha1.DeviceService{}

	for i := range edgeDeviceService {
		eds := edgeDeviceService[i]
		edName := util.GetEdgeDeviceServiceName(&eds, EdgeXObjectName)
		if _, exists := kubeDeviceService[edName]; !exists {
			redundantEdgeDeviceServices[edName] = ds.completeCreateContent(&eds)
		} else {
			kd := kubeDeviceService[edName]
			syncedDeviceServices[edName] = ds.completeUpdateContent(&kd, &eds)
		}
	}

	for i := range kubeDeviceService {
		kds := kubeDeviceService[i]
		if !kds.Status.Synced {
			continue
		}
		kdName := util.GetEdgeDeviceServiceName(&kds, EdgeXObjectName)
		if _, exists := edgeDeviceService[kdName]; !exists {
			redundantKubeDeviceServices[kdName] = &kds
		}
	}
	return
}

// syncEdgeToKube creates deviceServices on OpenYurt which are exists in edge platform but not in OpenYurt
func (ds *DeviceServiceList) syncEdgeToKube(edgeDevs map[string]*iotv1alpha1.DeviceService) error {
	for _, ed := range edgeDevs {
		if err := ds.Client.Create(context.TODO(), ed); err != nil {
			if apierrors.IsAlreadyExists(err) {
				klog.V(5).InfoS("DeviceService already exist on Kubernetes",
					"DeviceService", strings.ToLower(ed.Name))
				continue
			}
			klog.InfoS("created deviceService failed:", "DeviceService", strings.ToLower(ed.Name))
			return err
		}
	}
	return nil
}

// deleteDeviceServices deletes redundant deviceServices on OpenYurt
func (ds *DeviceServiceList) deleteDeviceServices(redundantKubeDeviceServices map[string]*iotv1alpha1.DeviceService) error {
	for _, kds := range redundantKubeDeviceServices {
		if err := ds.Client.Delete(context.TODO(), kds); err != nil {
			klog.V(5).ErrorS(err, "fail to delete the DeviceService on Kubernetes",
				"DeviceService", kds.Name)
			return err
		}
	}
	return nil
}

// updateDeviceServices updates deviceServices status on OpenYurt
func (ds *DeviceServiceList) updateDeviceServices(syncedDeviceServices map[string]*iotv1alpha1.DeviceService) error {
	for _, sd := range syncedDeviceServices {
		if sd.ObjectMeta.ResourceVersion == "" {
			continue
		}
		if err := ds.Client.Status().Update(context.TODO(), sd); err != nil {
			if apierrors.IsConflict(err) {
				klog.V(5).InfoS("update Conflicts", "DeviceService", sd.Name)
				continue
			}
			klog.V(5).ErrorS(err, "fail to update the DeviceService on Kubernetes",
				"DeviceService", sd.Name)
			return err
		}
	}
	return nil
}

// completeCreateContent completes the content of the deviceService which will be created on OpenYurt
func (ds *DeviceServiceList) completeCreateContent(edgeDS *iotv1alpha1.DeviceService) *iotv1alpha1.DeviceService {
	createDevice := edgeDS.DeepCopy()
	createDevice.Spec.NodePool = ds.NodePool
	createDevice.Namespace = ds.Namespace
	createDevice.Name = strings.Join([]string{ds.NodePool, edgeDS.Name}, "-")
	createDevice.Spec.Managed = false
	return createDevice
}

// completeUpdateContent completes the content of the deviceService which will be updated on OpenYurt
func (ds *DeviceServiceList) completeUpdateContent(kubeDS *iotv1alpha1.DeviceService, edgeDS *iotv1alpha1.DeviceService) *iotv1alpha1.DeviceService {
	updatedDS := kubeDS.DeepCopy()
	// update device status
	updatedDS.Status.LastConnected = edgeDS.Status.LastConnected
	updatedDS.Status.LastReported = edgeDS.Status.LastReported
	updatedDS.Status.AdminState = edgeDS.Status.AdminState
	return updatedDS
}

// DeviceProfileList
func (dps *DeviceProfileList) ProfileList() {

	// 1. get deviceProfiles on edge platform and OpenYurt
	edgeDeviceProfiles, kubeDeviceProfiles, err := dps.getAllDeviceProfiles()
	if err != nil {
		klog.V(3).ErrorS(err, "fail to list the deviceProfiles")
	}

	// 2. find the deviceProfiles that need to be synchronized
	redundantEdgeDeviceProfiles, redundantKubeDeviceProfiles, syncedDeviceProfiles :=
		dps.findDiffDeviceProfiles(edgeDeviceProfiles, kubeDeviceProfiles)
	klog.V(2).Infof("[DeviceProfile] The number of objects waiting for synchronization { %s:%d, %s:%d, %s:%d }",
		"Edge deviceProfiles should be added to OpenYurt", len(redundantEdgeDeviceProfiles),
		"OpenYurt deviceProfiles that should be deleted", len(redundantKubeDeviceProfiles),
		"DeviceProfiles that should be synchronized", len(syncedDeviceProfiles))

	// 3. create deviceProfiles on OpenYurt which are exists in edge platform but not in OpenYurt
	if err := dps.syncEdgeToKube(redundantEdgeDeviceProfiles); err != nil {
		klog.V(3).ErrorS(err, "fail to create deviceProfiles on OpenYurt")
	}

	// 4. delete redundant deviceProfiles on OpenYurt
	if err := dps.deleteDeviceProfiles(redundantKubeDeviceProfiles); err != nil {
		klog.V(3).ErrorS(err, "fail to delete redundant deviceProfiles on OpenYurt")
	}

	// 5. update deviceProfiles on OpenYurt
	// TODO

}

// Get the existing DeviceProfile on the Edge platform, as well as OpenYurt existing DeviceProfile
// edgeDeviceProfiles：map[actualName]DeviceProfile
// kubeDeviceProfiles：map[actualName]DeviceProfile
func (dps *DeviceProfileList) getAllDeviceProfiles() (
	map[string]iotv1alpha1.DeviceProfile, map[string]iotv1alpha1.DeviceProfile, error) {

	edgeDeviceProfiles := map[string]iotv1alpha1.DeviceProfile{}
	kubeDeviceProfiles := map[string]iotv1alpha1.DeviceProfile{}

	// 1. list deviceProfiles on edge platform
	eDps, err := dps.edgeClient.List(context.TODO(), devcli.ListOptions{Namespace: dps.Namespace})
	if err != nil {
		klog.V(4).ErrorS(err, "fail to list the deviceProfiles on the edge platform")
		return edgeDeviceProfiles, kubeDeviceProfiles, err
	}
	// 2. list deviceProfiles on OpenYurt (filter objects belonging to edgeServer)
	var kDps iotv1alpha1.DeviceProfileList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: dps.NodePool}
	if err = dps.List(context.TODO(), &kDps, listOptions, client.InNamespace(dps.Namespace)); err != nil {
		klog.V(4).ErrorS(err, "fail to list the deviceProfiles on the Kubernetes")
		return edgeDeviceProfiles, kubeDeviceProfiles, err
	}
	for i := range eDps {
		deviceProfilesName := util.GetEdgeDeviceProfileName(&eDps[i], EdgeXObjectName)
		edgeDeviceProfiles[deviceProfilesName] = eDps[i]
	}

	for i := range kDps.Items {
		deviceProfilesName := util.GetEdgeDeviceProfileName(&kDps.Items[i], EdgeXObjectName)
		kubeDeviceProfiles[deviceProfilesName] = kDps.Items[i]
	}
	return edgeDeviceProfiles, kubeDeviceProfiles, nil
}

// Get the list of deviceProfiles that need to be added, deleted and updated
func (dps *DeviceProfileList) findDiffDeviceProfiles(
	edgeDeviceProfiles map[string]iotv1alpha1.DeviceProfile, kubeDeviceProfiles map[string]iotv1alpha1.DeviceProfile) (
	redundantEdgeDeviceProfiles map[string]*iotv1alpha1.DeviceProfile, redundantKubeDeviceProfiles map[string]*iotv1alpha1.DeviceProfile, syncedDeviceProfiles map[string]*iotv1alpha1.DeviceProfile) {

	redundantEdgeDeviceProfiles = map[string]*iotv1alpha1.DeviceProfile{}
	redundantKubeDeviceProfiles = map[string]*iotv1alpha1.DeviceProfile{}
	syncedDeviceProfiles = map[string]*iotv1alpha1.DeviceProfile{}

	for i := range edgeDeviceProfiles {
		edp := edgeDeviceProfiles[i]
		edpName := util.GetEdgeDeviceProfileName(&edp, EdgeXObjectName)
		if _, exists := kubeDeviceProfiles[edpName]; !exists {
			redundantEdgeDeviceProfiles[edpName] = dps.completeCreateContent(&edp)
		} else {
			kdp := kubeDeviceProfiles[edpName]
			syncedDeviceProfiles[edpName] = dps.completeUpdateContent(&kdp, &edp)
		}
	}

	for i := range kubeDeviceProfiles {
		kdp := kubeDeviceProfiles[i]
		if !kdp.Status.Synced {
			continue
		}
		kdpName := util.GetEdgeDeviceProfileName(&kdp, EdgeXObjectName)
		if _, exists := edgeDeviceProfiles[kdpName]; !exists {
			redundantKubeDeviceProfiles[kdpName] = &kdp
		}
	}
	return
}

// completeCreateContent completes the content of the deviceProfile which will be created on OpenYurt
func (dps *DeviceProfileList) completeCreateContent(edgeDps *iotv1alpha1.DeviceProfile) *iotv1alpha1.DeviceProfile {
	createDeviceProfile := edgeDps.DeepCopy()
	createDeviceProfile.Namespace = dps.Namespace
	createDeviceProfile.Name = strings.Join([]string{dps.NodePool, createDeviceProfile.Name}, "-")
	createDeviceProfile.Spec.NodePool = dps.NodePool
	return createDeviceProfile
}

// completeUpdateContent completes the content of the deviceProfile which will be updated on OpenYurt
// TODO
func (dps *DeviceProfileList) completeUpdateContent(kubeDps *iotv1alpha1.DeviceProfile, edgeDS *iotv1alpha1.DeviceProfile) *iotv1alpha1.DeviceProfile {
	return kubeDps
}

// syncEdgeToKube creates deviceProfiles on OpenYurt which are exists in edge platform but not in OpenYurt
func (dps *DeviceProfileList) syncEdgeToKube(edgeDps map[string]*iotv1alpha1.DeviceProfile) error {
	for _, edp := range edgeDps {
		if err := dps.Client.Create(context.TODO(), edp); err != nil {
			if apierrors.IsAlreadyExists(err) {
				klog.V(5).Infof("DeviceProfile already exist on Kubernetes: %s", strings.ToLower(edp.Name))
				continue
			}
			klog.Infof("created deviceProfile failed: %s", strings.ToLower(edp.Name))
			return err
		}
	}
	return nil
}

// deleteDeviceProfiles deletes redundant deviceProfiles on OpenYurt
func (dps *DeviceProfileList) deleteDeviceProfiles(redundantKubeDeviceProfiles map[string]*iotv1alpha1.DeviceProfile) error {
	for _, kdp := range redundantKubeDeviceProfiles {
		if err := dps.Client.Delete(context.TODO(), kdp); err != nil {
			klog.V(5).ErrorS(err, "fail to delete the DeviceProfile on Kubernetes: %s ",
				"DeviceProfile", kdp.Name)
			return err
		}
	}
	return nil
}

func (help *ListHelper) list() {
	help.deviceServiceList.ServiceList()
	help.deviceProfileList.ProfileList()
}
