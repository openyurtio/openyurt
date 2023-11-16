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
	iotcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

type DeviceServiceSyncer struct {
	// Kubernetes client
	client.Client
	// syncing period in seconds
	syncPeriod       time.Duration
	deviceServiceCli iotcli.DeviceServiceInterface
	NodePool         string
	Namespace        string
}

func NewDeviceServiceSyncer(client client.Client, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) (DeviceServiceSyncer, error) {
	deviceserviceclient, err := edgexdock.CreateDeviceServiceClient()
	if err != nil {
		return DeviceServiceSyncer{}, err
	}
	return DeviceServiceSyncer{
		syncPeriod:       time.Duration(opts.EdgeSyncPeriod) * time.Second,
		deviceServiceCli: deviceserviceclient,
		Client:           client,
		NodePool:         opts.Nodepool,
		Namespace:        opts.Namespace,
	}, nil
}

func (ds *DeviceServiceSyncer) NewDeviceServiceSyncerRunnable() ctrlmgr.RunnableFunc {
	return func(ctx context.Context) error {
		ds.Run(ctx.Done())
		return nil
	}
}

func (ds *DeviceServiceSyncer) Run(stop <-chan struct{}) {
	klog.V(1).Info("[DeviceService] Starting the syncer...")
	go func() {
		for {
			<-time.After(ds.syncPeriod)
			klog.V(2).Info("[DeviceService] Start a round of synchronization.")
			// 1. get deviceServices on edge platform and OpenYurt
			edgeDeviceServices, kubeDeviceServices, err := ds.getAllDeviceServices()
			if err != nil {
				klog.V(3).ErrorS(err, "could not list the deviceServices")
				continue
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
				klog.V(3).ErrorS(err, "could not create deviceServices on OpenYurt")
			}

			// 4. delete redundant deviceServices on OpenYurt
			if err := ds.deleteDeviceServices(redundantKubeDeviceServices); err != nil {
				klog.V(3).ErrorS(err, "could not delete redundant deviceServices on OpenYurt")
			}

			// 5. update deviceService status on OpenYurt
			if err := ds.updateDeviceServices(syncedDeviceServices); err != nil {
				klog.V(3).ErrorS(err, "could not update deviceServices")
			}
			klog.V(2).Info("[DeviceService] One round of synchronization is complete")
		}
	}()

	<-stop
	klog.V(1).Info("[DeviceService] Stopping the syncer")
}

// Get the existing DeviceService on the Edge platform, as well as OpenYurt existing DeviceService
// edgeDeviceServices：map[actualName]DeviceService
// kubeDeviceServices：map[actualName]DeviceService
func (ds *DeviceServiceSyncer) getAllDeviceServices() (
	map[string]iotv1alpha1.DeviceService, map[string]iotv1alpha1.DeviceService, error) {

	edgeDeviceServices := map[string]iotv1alpha1.DeviceService{}
	kubeDeviceServices := map[string]iotv1alpha1.DeviceService{}

	// 1. list deviceServices on edge platform
	eDevSs, err := ds.deviceServiceCli.List(context.TODO(), iotcli.ListOptions{Namespace: ds.Namespace})
	if err != nil {
		klog.V(4).ErrorS(err, "could not list the deviceServices object on the edge platform")
		return edgeDeviceServices, kubeDeviceServices, err
	}
	// 2. list deviceServices on OpenYurt (filter objects belonging to edgeServer)
	var kDevSs iotv1alpha1.DeviceServiceList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: ds.NodePool}
	if err = ds.List(context.TODO(), &kDevSs, listOptions, client.InNamespace(ds.Namespace)); err != nil {
		klog.V(4).ErrorS(err, "could not list the deviceServices object on the Kubernetes")
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
func (ds *DeviceServiceSyncer) findDiffDeviceServices(
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
func (ds *DeviceServiceSyncer) syncEdgeToKube(edgeDevs map[string]*iotv1alpha1.DeviceService) error {
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
func (ds *DeviceServiceSyncer) deleteDeviceServices(redundantKubeDeviceServices map[string]*iotv1alpha1.DeviceService) error {
	for _, kds := range redundantKubeDeviceServices {
		if err := ds.Client.Delete(context.TODO(), kds); err != nil {
			klog.V(5).ErrorS(err, "could not delete the DeviceService on Kubernetes",
				"DeviceService", kds.Name)
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})
		if err := ds.Client.Patch(context.TODO(), kds, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			klog.V(5).ErrorS(err, "could not remove finalizer of DeviceService on Kubernetes", "DeviceService", kds.Name)
			return err
		}
	}
	return nil
}

// updateDeviceServices updates deviceServices status on OpenYurt
func (ds *DeviceServiceSyncer) updateDeviceServices(syncedDeviceServices map[string]*iotv1alpha1.DeviceService) error {
	for _, sd := range syncedDeviceServices {
		if sd.ObjectMeta.ResourceVersion == "" {
			continue
		}
		if err := ds.Client.Status().Update(context.TODO(), sd); err != nil {
			if apierrors.IsConflict(err) {
				klog.V(5).InfoS("update Conflicts", "DeviceService", sd.Name)
				continue
			}
			klog.V(5).ErrorS(err, "could not update the DeviceService on Kubernetes",
				"DeviceService", sd.Name)
			return err
		}
	}
	return nil
}

// completeCreateContent completes the content of the deviceService which will be created on OpenYurt
func (ds *DeviceServiceSyncer) completeCreateContent(edgeDS *iotv1alpha1.DeviceService) *iotv1alpha1.DeviceService {
	createDevice := edgeDS.DeepCopy()
	createDevice.Spec.NodePool = ds.NodePool
	createDevice.Namespace = ds.Namespace
	createDevice.Name = strings.Join([]string{ds.NodePool, edgeDS.Name}, "-")
	createDevice.Spec.Managed = false
	return createDevice
}

// completeUpdateContent completes the content of the deviceService which will be updated on OpenYurt
func (ds *DeviceServiceSyncer) completeUpdateContent(kubeDS *iotv1alpha1.DeviceService, edgeDS *iotv1alpha1.DeviceService) *iotv1alpha1.DeviceService {
	updatedDS := kubeDS.DeepCopy()
	// update device status
	updatedDS.Status.LastConnected = edgeDS.Status.LastConnected
	updatedDS.Status.LastReported = edgeDS.Status.LastReported
	updatedDS.Status.AdminState = edgeDS.Status.AdminState
	return updatedDS
}
