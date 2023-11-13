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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openyurtio/openyurt/cmd/yurt-iot-dock/app/options"
	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexobj "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

// DeviceProfileReconciler reconciles a DeviceProfile object
type DeviceProfileReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	edgeClient clients.DeviceProfileInterface
	NodePool   string
	Namespace  string
}

//+kubebuilder:rbac:groups=iot.openyurt.io,resources=deviceprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=iot.openyurt.io,resources=deviceprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=iot.openyurt.io,resources=deviceprofiles/finalizers,verbs=update

// Reconcile make changes to a deviceprofile object in EdgeX based on it in Kubernetes
func (r *DeviceProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dp iotv1alpha1.DeviceProfile
	if err := r.Get(ctx, req.NamespacedName, &dp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if dp.Spec.NodePool != r.NodePool {
		return ctrl.Result{}, nil
	}
	klog.V(3).Infof("Reconciling the DeviceProfile: %s", dp.GetName())

	// gets the actual name of deviceProfile on the edge platform from the Label of the deviceProfile
	dpActualName := util.GetEdgeDeviceProfileName(&dp, EdgeXObjectName)

	// 1. Handle the deviceProfile deletion event
	if err := r.reconcileDeleteDeviceProfile(ctx, &dp, dpActualName); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else if !dp.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !dp.Status.Synced {
		// 2. Synchronize OpenYurt deviceProfile to edge platform
		if err := r.reconcileCreateDeviceProfile(ctx, &dp, dpActualName); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			} else {
				return ctrl.Result{}, err
			}
		}
	}
	// 3. Handle the deviceProfile update event
	// TODO

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeviceProfileReconciler) SetupWithManager(mgr ctrl.Manager, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) error {
	deviceprofileclient, err := edgexdock.CreateDeviceProfileClient()
	if err != nil {
		return err
	}
	r.edgeClient = deviceprofileclient
	r.NodePool = opts.Nodepool
	r.Namespace = opts.Namespace

	return ctrl.NewControllerManagedBy(mgr).
		For(&iotv1alpha1.DeviceProfile{}).
		WithEventFilter(genFirstUpdateFilter("deviceprofile")).
		Complete(r)
}

func (r *DeviceProfileReconciler) reconcileDeleteDeviceProfile(ctx context.Context, dp *iotv1alpha1.DeviceProfile, actualName string) error {
	if dp.ObjectMeta.DeletionTimestamp.IsZero() {
		if len(dp.GetFinalizers()) == 0 {
			patchString := map[string]interface{}{
				"metadata": map[string]interface{}{
					"finalizers": []string{iotv1alpha1.DeviceProfileFinalizer},
				},
			}
			if patchData, err := json.Marshal(patchString); err != nil {
				return err
			} else {
				if err = r.Patch(ctx, dp, client.RawPatch(types.MergePatchType, patchData)); err != nil {
					return err
				}
			}
		}
	} else {
		patchString := map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		}
		// delete the deviceProfile in OpenYurt
		if patchData, err := json.Marshal(patchString); err != nil {
			return err
		} else {
			if err = r.Patch(ctx, dp, client.RawPatch(types.MergePatchType, patchData)); err != nil {
				return err
			}
		}

		// delete the deviceProfile object on edge platform
		err := r.edgeClient.Delete(context.TODO(), actualName, clients.DeleteOptions{})
		if err != nil && !clients.IsNotFoundErr(err) {
			return err
		}
	}
	return nil
}

func (r *DeviceProfileReconciler) reconcileCreateDeviceProfile(ctx context.Context, dp *iotv1alpha1.DeviceProfile, actualName string) error {
	klog.V(4).Infof("Checking if deviceProfile already exist on the edge platform: %s", dp.GetName())
	if edgeDp, err := r.edgeClient.Get(context.TODO(), actualName, clients.GetOptions{Namespace: r.Namespace}); err != nil {
		if !clients.IsNotFoundErr(err) {
			klog.V(4).ErrorS(err, "could not visit the edge platform")
			return nil
		}
	} else {
		// a. If object exists, the status of the deviceProfile on OpenYurt is updated
		klog.V(4).Info("DeviceProfile already exists on edge platform")
		dp.Status.Synced = true
		dp.Status.EdgeId = edgeDp.Status.EdgeId
		return r.Status().Update(ctx, dp)
	}

	// b. If object does not exist, a request is sent to the edge platform to create a new deviceProfile
	createDp, err := r.edgeClient.Create(context.Background(), dp, clients.CreateOptions{})
	if err != nil {
		klog.V(4).ErrorS(err, "failed to create deviceProfile on edge platform")
		return fmt.Errorf("failed to add deviceProfile to edge platform: %v", err)
	}
	klog.V(3).Infof("Successfully add DeviceProfile to edge platform, Name: %s, EdgeId: %s", createDp.GetName(), createDp.Status.EdgeId)
	dp.Status.EdgeId = createDp.Status.EdgeId
	dp.Status.Synced = true
	return r.Status().Update(ctx, dp)
}

func DeleteDeviceProfilesOnControllerShutdown(ctx context.Context, cli client.Client, opts *options.YurtIoTDockOptions) error {
	var deviceProfileList iotv1alpha1.DeviceProfileList
	if err := cli.List(ctx, &deviceProfileList, client.InNamespace(opts.Namespace)); err != nil {
		return err
	}
	klog.V(4).Infof("DeviceProfileList, successfully get the list")

	for _, deviceProfile := range deviceProfileList.Items {
		controllerutil.RemoveFinalizer(&deviceProfile, iotv1alpha1.DeviceProfileFinalizer)
		if err := cli.Update(ctx, &deviceProfile); err != nil {
			klog.Errorf("deviceProfileName: %s, update deviceProfile err:%v", deviceProfile.GetName(), err)
			continue
		}

		if err := cli.Delete(ctx, &deviceProfile); err != nil {
			klog.Errorf("deviceProfileName: %s, update deviceProfile err:%v", deviceProfile.GetName(), err)
			continue
		}
	}
	klog.V(4).Infof("DeviceProfileList, successfully delete the list")

	return nil
}
