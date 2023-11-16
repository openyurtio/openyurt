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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	edgeCli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

type DeviceSyncer struct {
	// kubernetes client
	client.Client
	// which nodePool deviceController is deployed in
	NodePool string
	// edge platform's client
	deviceCli edgeCli.DeviceInterface
	// syncing period in seconds
	syncPeriod time.Duration
	Namespace  string
}

// NewDeviceSyncer initialize a New DeviceSyncer
func NewDeviceSyncer(client client.Client, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) (DeviceSyncer, error) {
	devicelient, err := edgexdock.CreateDeviceClient()
	if err != nil {
		return DeviceSyncer{}, err
	}
	return DeviceSyncer{
		syncPeriod: time.Duration(opts.EdgeSyncPeriod) * time.Second,
		deviceCli:  devicelient,
		Client:     client,
		NodePool:   opts.Nodepool,
		Namespace:  opts.Namespace,
	}, nil
}

// NewDeviceSyncerRunnable initialize a controller-runtime manager runnable
func (ds *DeviceSyncer) NewDeviceSyncerRunnable() ctrlmgr.RunnableFunc {
	return func(ctx context.Context) error {
		ds.Run(ctx.Done())
		return nil
	}
}

func (ds *DeviceSyncer) Run(stop <-chan struct{}) {
	klog.V(1).Info("[Device] Starting the syncer...")
	go func() {
		for {
			<-time.After(ds.syncPeriod)
			klog.V(2).Info("[Device] Start a round of synchronization.")
			// 1. get device on edge platform and OpenYurt
			edgeDevices, kubeDevices, err := ds.getAllDevices()
			if err != nil {
				klog.V(3).ErrorS(err, "could not list the devices")
				continue
			}

			// 2. find the device that need to be synchronized
			redundantEdgeDevices, redundantKubeDevices, syncedDevices := ds.findDiffDevice(edgeDevices, kubeDevices)
			klog.V(2).Infof("[Device] The number of objects waiting for synchronization { %s:%d, %s:%d, %s:%d }",
				"Edge device should be added to OpenYurt", len(redundantEdgeDevices),
				"OpenYurt device that should be deleted", len(redundantKubeDevices),
				"Devices that should be synchronized", len(syncedDevices))

			// 3. create device on OpenYurt which are exists in edge platform but not in OpenYurt
			if err := ds.syncEdgeToKube(redundantEdgeDevices); err != nil {
				klog.V(3).ErrorS(err, "could not create devices on OpenYurt")
			}

			// 4. delete redundant device on OpenYurt
			if err := ds.deleteDevices(redundantKubeDevices); err != nil {
				klog.V(3).ErrorS(err, "could not delete redundant devices on OpenYurt")
			}

			// 5. update device status on OpenYurt
			if err := ds.updateDevices(syncedDevices); err != nil {
				klog.V(3).ErrorS(err, "could not update devices status")
			}
			klog.V(2).Info("[Device] One round of synchronization is complete")
		}
	}()

	<-stop
	klog.V(1).Info("[Device] Stopping the syncer")
}

// Get the existing Device on the Edge platform, as well as OpenYurt existing Device
// edgeDevice：map[actualName]device
// kubeDevice：map[actualName]device
func (ds *DeviceSyncer) getAllDevices() (map[string]iotv1alpha1.Device, map[string]iotv1alpha1.Device, error) {
	edgeDevice := map[string]iotv1alpha1.Device{}
	kubeDevice := map[string]iotv1alpha1.Device{}
	// 1. list devices on edge platform
	eDevs, err := ds.deviceCli.List(context.TODO(), edgeCli.ListOptions{Namespace: ds.Namespace})
	if err != nil {
		klog.V(4).ErrorS(err, "could not list the devices object on the Edge Platform")
		return edgeDevice, kubeDevice, err
	}
	// 2. list devices on OpenYurt (filter objects belonging to edgeServer)
	var kDevs iotv1alpha1.DeviceList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: ds.NodePool}
	if err = ds.List(context.TODO(), &kDevs, listOptions, client.InNamespace(ds.Namespace)); err != nil {
		klog.V(4).ErrorS(err, "could not list the devices object on the OpenYurt")
		return edgeDevice, kubeDevice, err
	}
	for i := range eDevs {
		deviceName := util.GetEdgeDeviceName(&eDevs[i], EdgeXObjectName)
		edgeDevice[deviceName] = eDevs[i]
	}

	for i := range kDevs.Items {
		deviceName := util.GetEdgeDeviceName(&kDevs.Items[i], EdgeXObjectName)
		kubeDevice[deviceName] = kDevs.Items[i]
	}
	return edgeDevice, kubeDevice, nil
}

// Get the list of devices that need to be added, deleted and updated
func (ds *DeviceSyncer) findDiffDevice(
	edgeDevices map[string]iotv1alpha1.Device, kubeDevices map[string]iotv1alpha1.Device) (
	redundantEdgeDevices map[string]*iotv1alpha1.Device, redundantKubeDevices map[string]*iotv1alpha1.Device, syncedDevices map[string]*iotv1alpha1.Device) {

	redundantEdgeDevices = map[string]*iotv1alpha1.Device{}
	redundantKubeDevices = map[string]*iotv1alpha1.Device{}
	syncedDevices = map[string]*iotv1alpha1.Device{}

	for i := range edgeDevices {
		ed := edgeDevices[i]
		edName := util.GetEdgeDeviceName(&ed, EdgeXObjectName)
		if _, exists := kubeDevices[edName]; !exists {
			klog.V(5).Infof("found redundant edge device %s", edName)
			redundantEdgeDevices[edName] = ds.completeCreateContent(&ed)
		} else {
			klog.V(5).Infof("found device %s to be synced", edName)
			kd := kubeDevices[edName]
			syncedDevices[edName] = ds.completeUpdateContent(&kd, &ed)
		}
	}

	for i := range kubeDevices {
		kd := kubeDevices[i]
		if !kd.Status.Synced {
			continue
		}
		kdName := util.GetEdgeDeviceName(&kd, EdgeXObjectName)
		if _, exists := edgeDevices[kdName]; !exists {
			redundantKubeDevices[kdName] = &kd
		}
	}
	return
}

// syncEdgeToKube creates device on OpenYurt which are exists in edge platform but not in OpenYurt
func (ds *DeviceSyncer) syncEdgeToKube(edgeDevs map[string]*iotv1alpha1.Device) error {
	for _, ed := range edgeDevs {
		if err := ds.Client.Create(context.TODO(), ed); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			klog.V(5).ErrorS(err, "could not create device on OpenYurt", "DeviceName", strings.ToLower(ed.Name))
			return err
		}
	}
	return nil
}

// deleteDevices deletes redundant device on OpenYurt
func (ds *DeviceSyncer) deleteDevices(redundantKubeDevices map[string]*iotv1alpha1.Device) error {
	for _, kd := range redundantKubeDevices {
		if err := ds.Client.Delete(context.TODO(), kd); err != nil {
			klog.V(5).ErrorS(err, "could not delete the device on OpenYurt",
				"DeviceName", kd.Name)
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})
		if err := ds.Client.Patch(context.TODO(), kd, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			klog.V(5).ErrorS(err, "could not remove finalizer of Device on Kubernetes", "Device", kd.Name)
			return err
		}
	}
	return nil
}

// updateDevicesStatus updates device status on OpenYurt
func (ds *DeviceSyncer) updateDevices(syncedDevices map[string]*iotv1alpha1.Device) error {
	for n := range syncedDevices {
		if err := ds.Client.Status().Update(context.TODO(), syncedDevices[n]); err != nil {
			if apierrors.IsConflict(err) {
				klog.V(5).InfoS("update Conflicts", "Device", syncedDevices[n].Name)
				continue
			}
			return err
		}
	}
	return nil
}

// completeCreateContent completes the content of the device which will be created on OpenYurt
func (ds *DeviceSyncer) completeCreateContent(edgeDevice *iotv1alpha1.Device) *iotv1alpha1.Device {
	createDevice := edgeDevice.DeepCopy()
	createDevice.Spec.NodePool = ds.NodePool
	createDevice.Name = strings.Join([]string{ds.NodePool, createDevice.Name}, "-")
	createDevice.Namespace = ds.Namespace
	createDevice.Spec.Managed = false

	return createDevice
}

// completeUpdateContent completes the content of the device which will be updated on OpenYurt
func (ds *DeviceSyncer) completeUpdateContent(kubeDevice *iotv1alpha1.Device, edgeDevice *iotv1alpha1.Device) *iotv1alpha1.Device {
	updatedDevice := kubeDevice.DeepCopy()
	_, aps, _ := ds.deviceCli.ListPropertiesState(context.TODO(), updatedDevice, edgeCli.ListOptions{})
	// update device status
	updatedDevice.Status.LastConnected = edgeDevice.Status.LastConnected
	updatedDevice.Status.LastReported = edgeDevice.Status.LastReported
	updatedDevice.Status.AdminState = edgeDevice.Status.AdminState
	updatedDevice.Status.OperatingState = edgeDevice.Status.OperatingState
	updatedDevice.Status.DeviceProperties = aps
	return updatedDevice
}
