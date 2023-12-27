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
	"time"

	corev1 "k8s.io/api/core/v1"
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
	util "github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

// DeviceReconciler reconciles a Device object
type DeviceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	deviceCli clients.DeviceInterface
	// which nodePool deviceController is deployed in
	NodePool  string
	Namespace string
}

//+kubebuilder:rbac:groups=iot.openyurt.io,resources=devices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=iot.openyurt.io,resources=devices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=iot.openyurt.io,resources=devices/finalizers,verbs=update

func (r *DeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var d iotv1alpha1.Device
	if err := r.Get(ctx, req.NamespacedName, &d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If objects doesn't belong to the Edge platform to which the controller is connected, the controller does not handle events for that object
	if d.Spec.NodePool != r.NodePool {
		return ctrl.Result{}, nil
	}
	klog.V(3).Infof("Reconciling the Device: %s", d.GetName())

	deviceStatus := d.Status.DeepCopy()
	// Update the conditions for device
	defer func() {
		if !d.Spec.Managed {
			util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceManagingCondition, corev1.ConditionFalse, iotv1alpha1.DeviceManagingReason, ""))
		}

		err := r.Status().Update(ctx, &d)
		if client.IgnoreNotFound(err) != nil {
			if !apierrors.IsConflict(err) {
				klog.V(4).ErrorS(err, "update device conditions failed", "DeviceName", d.GetName())
			}
		}
	}()

	// 1. Handle the device deletion event
	if err := r.reconcileDeleteDevice(ctx, &d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else if !d.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !d.Status.Synced {
		// 2. Synchronize OpenYurt device objects to edge platform
		if err := r.reconcileCreateDevice(ctx, &d, deviceStatus); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			} else {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	} else if d.Spec.Managed {
		// 3. If the device has been synchronized and is managed by the cloud, reconcile the device properties
		if err := r.reconcileUpdateDevice(ctx, &d, deviceStatus); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{RequeueAfter: time.Second * 2}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeviceReconciler) SetupWithManager(mgr ctrl.Manager, opts *options.YurtIoTDockOptions, edgexdock *edgexobj.EdgexDock) error {
	deviceclient, err := edgexdock.CreateDeviceClient()
	if err != nil {
		return err
	}
	r.deviceCli = deviceclient
	r.NodePool = opts.Nodepool
	r.Namespace = opts.Namespace

	return ctrl.NewControllerManagedBy(mgr).
		For(&iotv1alpha1.Device{}).
		WithEventFilter(genFirstUpdateFilter("device")).
		Complete(r)
}

func (r *DeviceReconciler) reconcileDeleteDevice(ctx context.Context, d *iotv1alpha1.Device) error {
	// gets the actual name of the device on the Edge platform from the Label of the device
	edgeDeviceName := util.GetEdgeDeviceName(d, EdgeXObjectName)
	if d.ObjectMeta.DeletionTimestamp.IsZero() {
		if len(d.GetFinalizers()) == 0 {
			patchData, _ := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"finalizers": []string{iotv1alpha1.DeviceFinalizer},
				},
			})
			if err := r.Patch(ctx, d, client.RawPatch(types.MergePatchType, patchData)); err != nil {
				return err
			}
		}
	} else {
		// delete the device object on the edge platform
		err := r.deviceCli.Delete(context.TODO(), edgeDeviceName, clients.DeleteOptions{})
		if err != nil && !clients.IsNotFoundErr(err) {
			return err
		}

		// delete the device in OpenYurt
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"finalizers": []string{},
			},
		})
		if err = r.Patch(ctx, d, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			return err
		}
	}
	return nil
}

func (r *DeviceReconciler) reconcileCreateDevice(ctx context.Context, d *iotv1alpha1.Device, deviceStatus *iotv1alpha1.DeviceStatus) error {
	// get the actual name of the device on the Edge platform from the Label of the device
	edgeDeviceName := util.GetEdgeDeviceName(d, EdgeXObjectName)
	newDeviceStatus := d.Status.DeepCopy()
	klog.V(4).Infof("Checking if device already exist on the edge platform: %s", d.GetName())
	// Checking if device already exist on the edge platform
	edgeDevice, err := r.deviceCli.Get(context.TODO(), edgeDeviceName, clients.GetOptions{Namespace: r.Namespace})
	if err == nil {
		// a. If object exists, the status of the device on OpenYurt is updated
		klog.V(4).Infof("Device already exists on edge platform: %s", d.GetName())
		newDeviceStatus.EdgeId = edgeDevice.Status.EdgeId
		newDeviceStatus.Synced = true
	} else if clients.IsNotFoundErr(err) {
		// b. If the object does not exist, a request is sent to the edge platform to create a new device
		klog.V(4).Infof("Adding device to the edge platform: %s", d.GetName())
		createdEdgeObj, err := r.deviceCli.Create(context.TODO(), d, clients.CreateOptions{})
		if err != nil {
			util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceSyncedCondition, corev1.ConditionFalse, iotv1alpha1.DeviceCreateSyncedReason, err.Error()))
			return fmt.Errorf("could not add Device to edge platform: %v", err)
		} else {
			klog.V(4).Infof("Successfully add Device to edge platform, Name: %s, EdgeId: %s", edgeDeviceName, createdEdgeObj.Status.EdgeId)
			newDeviceStatus.EdgeId = createdEdgeObj.Status.EdgeId
			newDeviceStatus.Synced = true
		}
	} else {
		klog.V(4).ErrorS(err, "could not visit the edge platform")
		util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceSyncedCondition, corev1.ConditionFalse, iotv1alpha1.DeviceVistedCoreMetadataSyncedReason, ""))
		return nil
	}
	d.Status = *newDeviceStatus
	util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceSyncedCondition, corev1.ConditionTrue, "", ""))

	return r.Status().Update(ctx, d)
}

func (r *DeviceReconciler) reconcileUpdateDevice(ctx context.Context, d *iotv1alpha1.Device, deviceStatus *iotv1alpha1.DeviceStatus) error {
	// the device has been added to the edge platform, check if each device property are in the desired state
	newDeviceStatus := d.Status.DeepCopy()
	// This list is used to hold the names of properties that failed to reconcile
	var failedPropertyNames []string

	// 1. reconciling the AdminState and OperatingState field of device
	klog.V(3).Infof("DeviceName: %s, reconciling the AdminState and OperatingState field of device", d.GetName())
	updateDevice := d.DeepCopy()
	if d.Spec.AdminState != "" && d.Spec.AdminState != d.Status.AdminState {
		newDeviceStatus.AdminState = d.Spec.AdminState
	} else {
		updateDevice.Spec.AdminState = ""
	}

	if d.Spec.OperatingState != "" && d.Spec.OperatingState != d.Status.OperatingState {
		newDeviceStatus.OperatingState = d.Spec.OperatingState
	} else {
		updateDevice.Spec.OperatingState = ""
	}
	_, err := r.deviceCli.Update(context.TODO(), updateDevice, clients.UpdateOptions{})
	if err != nil {
		util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceManagingCondition, corev1.ConditionFalse, iotv1alpha1.DeviceUpdateStateReason, err.Error()))
		return err
	}

	// 2. reconciling the device properties' value
	klog.V(3).Infof("DeviceName: %s, reconciling the device properties", d.GetName())
	// property updates are made only when the device is up and unlocked
	if newDeviceStatus.OperatingState == iotv1alpha1.Up && newDeviceStatus.AdminState == iotv1alpha1.UnLocked {
		newDeviceStatus, failedPropertyNames = r.reconcileDeviceProperties(d, newDeviceStatus)
	}

	d.Status = *newDeviceStatus

	// 3. update the device status on OpenYurt
	klog.V(3).Infof("DeviceName: %s, update the device status", d.GetName())
	if err := r.Status().Update(ctx, d); err != nil {
		util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceManagingCondition, corev1.ConditionFalse, iotv1alpha1.DeviceUpdateStateReason, err.Error()))
		return err
	} else if len(failedPropertyNames) != 0 {
		err = fmt.Errorf("the following device properties could not reconcile: %v", failedPropertyNames)
		util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceManagingCondition, corev1.ConditionFalse, err.Error(), ""))
		return nil
	}

	util.SetDeviceCondition(deviceStatus, util.NewDeviceCondition(iotv1alpha1.DeviceManagingCondition, corev1.ConditionTrue, "", ""))
	return nil
}

// Update the actual property value of the device on edge platform,
// return the latest status and the names of the property that failed to update
func (r *DeviceReconciler) reconcileDeviceProperties(d *iotv1alpha1.Device, deviceStatus *iotv1alpha1.DeviceStatus) (*iotv1alpha1.DeviceStatus, []string) {
	newDeviceStatus := deviceStatus.DeepCopy()
	// This list is used to hold the names of properties that failed to reconcile
	var failedPropertyNames []string
	// 2. reconciling the device properties' value
	klog.V(3).Infof("DeviceName: %s, reconciling the value of device properties", d.GetName())
	for _, desiredProperty := range d.Spec.DeviceProperties {
		if desiredProperty.DesiredValue == "" {
			continue
		}
		propertyName := desiredProperty.Name
		// 1.1. gets the actual property value of the current device from edge platform
		klog.V(4).Infof("DeviceName: %s, getting the actual value of property: %s", d.GetName(), propertyName)
		actualProperty, err := r.deviceCli.GetPropertyState(context.TODO(), propertyName, d, clients.GetOptions{})
		if err != nil {
			if !clients.IsNotFoundErr(err) {
				klog.Errorf("DeviceName: %s, could not get actual property value of %s, err:%v", d.GetName(), propertyName, err)
				failedPropertyNames = append(failedPropertyNames, propertyName)
				continue
			}
			klog.Errorf("DeviceName: %s, property read command not found", d.GetName())
		} else {
			klog.V(4).Infof("DeviceName: %s, got the actual property state, {Name: %s, GetURL: %s, ActualValue: %s}",
				d.GetName(), propertyName, actualProperty.GetURL, actualProperty.ActualValue)
		}

		if newDeviceStatus.DeviceProperties == nil {
			newDeviceStatus.DeviceProperties = map[string]iotv1alpha1.ActualPropertyState{}
		} else {
			newDeviceStatus.DeviceProperties[propertyName] = *actualProperty
		}

		// 1.2. set the device attribute in the edge platform to the expected value
		var actualValue string
		if actualProperty != nil {
			if actualValue = actualProperty.ActualValue; desiredProperty.DesiredValue == actualValue {
				// The actual value is same as the desired one.
				continue
			}
		}

		klog.V(4).Infof("DeviceName: %s, the desired value and the actual value are different, desired: %s, actual: %s",
			d.GetName(), desiredProperty.DesiredValue, actualValue)

		if err := r.deviceCli.UpdatePropertyState(context.TODO(), propertyName, d, clients.UpdateOptions{}); err != nil {
			klog.ErrorS(err, "could not update property", "DeviceName", d.GetName(), "propertyName", propertyName)
			failedPropertyNames = append(failedPropertyNames, propertyName)
			continue
		}

		klog.V(4).Infof("DeviceName: %s, successfully set the property %s to desired value", d.GetName(), propertyName)
		newActualProperty := iotv1alpha1.ActualPropertyState{
			Name:        propertyName,
			GetURL:      desiredProperty.PutURL,
			ActualValue: desiredProperty.DesiredValue,
		}
		newDeviceStatus.DeviceProperties[propertyName] = newActualProperty
	}
	return newDeviceStatus, failedPropertyNames
}

func DeleteDevicesOnControllerShutdown(ctx context.Context, cli client.Client, opts *options.YurtIoTDockOptions) error {
	var deviceList iotv1alpha1.DeviceList
	if err := cli.List(ctx, &deviceList, client.InNamespace(opts.Namespace)); err != nil {
		return err
	}
	klog.V(4).Infof("DeviceList, successfully get the list")

	for _, device := range deviceList.Items {
		controllerutil.RemoveFinalizer(&device, iotv1alpha1.DeviceFinalizer)
		if err := cli.Update(ctx, &device); err != nil {
			klog.Errorf("DeviceName: %s, update device err:%v", device.GetName(), err)
			continue
		}

		if err := cli.Delete(ctx, &device); err != nil {
			klog.Errorf("DeviceName: %s, update device err:%v", device.GetName(), err)
			continue
		}
	}
	klog.V(4).Infof("DeviceList, successfully delete the list")

	return nil
}
