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
	devcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

type DeviceProfileSyncer struct {
	// syncing period in seconds
	syncPeriod time.Duration
	// edge platform client
	edgeClient devcli.DeviceProfileInterface
	// Kubernetes client
	client.Client
	NodePool  string
	Namespace string
}

// NewDeviceProfileSyncer initialize a New DeviceProfileSyncer
func NewDeviceProfileSyncer(client client.Client, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) (DeviceProfileSyncer, error) {
	edgeclient, err := edgexdock.CreateDeviceProfileClient()
	if err != nil {
		return DeviceProfileSyncer{}, err
	}
	return DeviceProfileSyncer{
		syncPeriod: time.Duration(opts.EdgeSyncPeriod) * time.Second,
		edgeClient: edgeclient,
		Client:     client,
		NodePool:   opts.Nodepool,
		Namespace:  opts.Namespace,
	}, nil
}

// NewDeviceProfileSyncerRunnable initialize a controller-runtime manager runnable
func (dps *DeviceProfileSyncer) NewDeviceProfileSyncerRunnable() ctrlmgr.RunnableFunc {
	return func(ctx context.Context) error {
		dps.Run(ctx.Done())
		return nil
	}
}

func (dps *DeviceProfileSyncer) Run(stop <-chan struct{}) {
	klog.V(1).Info("[DeviceProfile] Starting the syncer...")
	go func() {
		for {
			<-time.After(dps.syncPeriod)
			klog.V(2).Info("[DeviceProfile] Start a round of synchronization.")

			// 1. get deviceProfiles on edge platform and OpenYurt
			edgeDeviceProfiles, kubeDeviceProfiles, err := dps.getAllDeviceProfiles()
			if err != nil {
				klog.V(3).ErrorS(err, "could not list the deviceProfiles")
				continue
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
				klog.V(3).ErrorS(err, "could not create deviceProfiles on OpenYurt")
			}

			// 4. delete redundant deviceProfiles on OpenYurt
			if err := dps.deleteDeviceProfiles(redundantKubeDeviceProfiles); err != nil {
				klog.V(3).ErrorS(err, "could not delete redundant deviceProfiles on OpenYurt")
			}

			// 5. update deviceProfiles on OpenYurt
			// TODO
		}
	}()

	<-stop
	klog.V(1).Info("[DeviceProfile] Stopping the syncer")
}

// Get the existing DeviceProfile on the Edge platform, as well as OpenYurt existing DeviceProfile
// edgeDeviceProfiles：map[actualName]DeviceProfile
// kubeDeviceProfiles：map[actualName]DeviceProfile
func (dps *DeviceProfileSyncer) getAllDeviceProfiles() (
	map[string]iotv1alpha1.DeviceProfile, map[string]iotv1alpha1.DeviceProfile, error) {

	edgeDeviceProfiles := map[string]iotv1alpha1.DeviceProfile{}
	kubeDeviceProfiles := map[string]iotv1alpha1.DeviceProfile{}

	// 1. list deviceProfiles on edge platform
	eDps, err := dps.edgeClient.List(context.TODO(), devcli.ListOptions{Namespace: dps.Namespace})
	if err != nil {
		klog.V(4).ErrorS(err, "could not list the deviceProfiles on the edge platform")
		return edgeDeviceProfiles, kubeDeviceProfiles, err
	}
	// 2. list deviceProfiles on OpenYurt (filter objects belonging to edgeServer)
	var kDps iotv1alpha1.DeviceProfileList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: dps.NodePool}
	if err = dps.List(context.TODO(), &kDps, listOptions, client.InNamespace(dps.Namespace)); err != nil {
		klog.V(4).ErrorS(err, "could not list the deviceProfiles on the Kubernetes")
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
func (dps *DeviceProfileSyncer) findDiffDeviceProfiles(
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
func (dps *DeviceProfileSyncer) completeCreateContent(edgeDps *iotv1alpha1.DeviceProfile) *iotv1alpha1.DeviceProfile {
	createDeviceProfile := edgeDps.DeepCopy()
	createDeviceProfile.Namespace = dps.Namespace
	createDeviceProfile.Name = strings.Join([]string{dps.NodePool, createDeviceProfile.Name}, "-")
	createDeviceProfile.Spec.NodePool = dps.NodePool
	return createDeviceProfile
}

// completeUpdateContent completes the content of the deviceProfile which will be updated on OpenYurt
// TODO
func (dps *DeviceProfileSyncer) completeUpdateContent(kubeDps *iotv1alpha1.DeviceProfile, edgeDS *iotv1alpha1.DeviceProfile) *iotv1alpha1.DeviceProfile {
	return kubeDps
}

// syncEdgeToKube creates deviceProfiles on OpenYurt which are exists in edge platform but not in OpenYurt
func (dps *DeviceProfileSyncer) syncEdgeToKube(edgeDps map[string]*iotv1alpha1.DeviceProfile) error {
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
func (dps *DeviceProfileSyncer) deleteDeviceProfiles(redundantKubeDeviceProfiles map[string]*iotv1alpha1.DeviceProfile) error {
	for _, kdp := range redundantKubeDeviceProfiles {
		if err := dps.Client.Delete(context.TODO(), kdp); err != nil {
			klog.V(5).ErrorS(err, "could not delete the DeviceProfile on Kubernetes: %s ",
				"DeviceProfile", kdp.Name)
			return err
		}
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})
		if err := dps.Client.Patch(context.TODO(), kdp, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			klog.V(5).ErrorS(err, "could not remove finalizer of DeviceProfile on Kubernetes", "DeviceProfile", kdp.Name)
			return err
		}
	}
	return nil
}
