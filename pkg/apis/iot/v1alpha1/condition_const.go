/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

const (
	// ConfigmapAvailableCondition documents the status of the PlatformAdmin configmap.
	ConfigmapAvailableCondition PlatformAdminConditionType = "ConfigmapAvailable"

	ConfigmapProvisioningReason = "ConfigmapProvisioning"

	ConfigmapProvisioningFailedReason = "ConfigmapProvisioningFailed"
	// ServiceAvailableCondition documents the status of the PlatformAdmin service.
	ServiceAvailableCondition PlatformAdminConditionType = "ServiceAvailable"

	ServiceProvisioningReason = "ServiceProvisioning"

	ServiceProvisioningFailedReason = "ServiceProvisioningFailed"
	// DeploymentAvailableCondition documents the status of the PlatformAdmin deployment.
	DeploymentAvailableCondition PlatformAdminConditionType = "DeploymentAvailable"

	DeploymentProvisioningReason = "DeploymentProvisioning"

	DeploymentProvisioningFailedReason = "DeploymentProvisioningFailed"

	// DeviceSyncedCondition indicates that the device exists in both OpenYurt and edge platform
	DeviceSyncedCondition DeviceConditionType = "DeviceSynced"

	DeviceManagingReason = "This device is not managed by openyurt"

	DeviceCreateSyncedReason = "Failed to create device on edge platform"

	// DeviceManagingCondition indicates that the device is being managed by cloud and its properties are being reconciled
	DeviceManagingCondition DeviceConditionType = "DeviceManaging"

	DeviceVistedCoreMetadataSyncedReason = "Failed to visit the EdgeX core-metadata-service"

	DeviceUpdateStateReason = "Failed to update AdminState or OperatingState of device on edge platform"

	// DeviceServiceSyncedCondition indicates that the deviceService exists in both OpenYurt and edge platform
	DeviceServiceSyncedCondition DeviceServiceConditionType = "DeviceServiceSynced"

	DeviceServiceManagingReason = "This deviceService is not managed by openyurt"

	// DeviceServiceManagingCondition indicates that the deviceService is being managed by cloud and its field are being reconciled
	DeviceServiceManagingCondition DeviceServiceConditionType = "DeviceServiceManaging"

	DeviceServiceCreateSyncedReason = "Failed to add DeviceService to EdgeX"

	DeviceServiceUpdateStatusSyncedReason = "Failed to update DeviceService status"
)
