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

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeviceFinalizer = "iot.openyurt.io/device"
)

// DeviceConditionType indicates valid conditions type of a Device.
type DeviceConditionType string

type AdminState string

const (
	Locked   AdminState = "LOCKED"
	UnLocked AdminState = "UNLOCKED"
)

type OperatingState string

const (
	Unknown OperatingState = "UNKNOWN"
	Up      OperatingState = "UP"
	Down    OperatingState = "DOWN"
)

type ProtocolProperties map[string]string

// DeviceSpec defines the desired state of Device
type DeviceSpec struct {
	// Information describing the device
	Description string `json:"description,omitempty"`
	// Admin state (locked/unlocked)
	AdminState AdminState `json:"adminState,omitempty"`
	// Operating state (enabled/disabled)
	OperatingState OperatingState `json:"operatingState,omitempty"`
	// A map of supported protocols for the given device
	Protocols map[string]ProtocolProperties `json:"protocols,omitempty"`
	// Other labels applied to the device to help with searching
	Labels []string `json:"labels,omitempty"`
	// Device service specific location (interface{} is an empty interface so
	// it can be anything)
	// TODO: location type in edgex is interface{}
	Location string `json:"location,omitempty"`
	// Associated Device Service - One per device
	Service string `json:"serviceName"`
	// Associated Device Profile - Describes the device
	Profile string `json:"profileName"`
	Notify  bool   `json:"notify"`
	// True means device is managed by cloud, cloud can update the related fields
	// False means cloud can't update the fields
	Managed bool `json:"managed,omitempty"`
	// NodePool indicates which nodePool the device comes from
	NodePool string `json:"nodePool,omitempty"`
	// TODO support the following field
	// A list of auto-generated events coming from the device
	// AutoEvents     []AutoEvent                   `json:"autoEvents"`
	// DeviceProperties represents the expected state of the device's properties
	DeviceProperties map[string]DesiredPropertyState `json:"deviceProperties,omitempty"`
}

type DesiredPropertyState struct {
	Name         string `json:"name"`
	PutURL       string `json:"putURL,omitempty"`
	DesiredValue string `json:"desiredValue"`
}

type ActualPropertyState struct {
	Name        string `json:"name"`
	GetURL      string `json:"getURL,omitempty"`
	ActualValue string `json:"actualValue"`
}

// DeviceStatus defines the observed state of Device
type DeviceStatus struct {
	// Time (milliseconds) that the device last provided any feedback or
	// responded to any request
	LastConnected int64 `json:"lastConnected,omitempty"`
	// Time (milliseconds) that the device reported data to the core
	// microservice
	LastReported int64 `json:"lastReported,omitempty"`
	// Synced indicates whether the device already exists on both OpenYurt and edge platform
	Synced bool `json:"synced,omitempty"`
	// it represents the actual state of the device's properties
	DeviceProperties map[string]ActualPropertyState `json:"deviceProperties,omitempty"`
	EdgeId           string                         `json:"edgeId,omitempty"`
	// Admin state (locked/unlocked)
	AdminState AdminState `json:"adminState,omitempty"`
	// Operating state (up/down/unknown)
	OperatingState OperatingState `json:"operatingState,omitempty"`
	// current device state
	// +optional
	Conditions []DeviceCondition `json:"conditions,omitempty"`
}

// DeviceCondition describes current state of a Device.
type DeviceCondition struct {
	// Type of in place set condition.
	Type DeviceConditionType `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dev
// +kubebuilder:printcolumn:name="NODEPOOL",type="string",JSONPath=".spec.nodePool",description="The nodepool of device"
// +kubebuilder:printcolumn:name="SYNCED",type="boolean",JSONPath=".status.synced",description="The synced status of device"
// +kubebuilder:printcolumn:name="MANAGED",type="boolean",priority=1,JSONPath=".spec.managed",description="The managed status of device"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// Device is the Schema for the devices API
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSpec   `json:"spec,omitempty"`
	Status DeviceStatus `json:"status,omitempty"`
}

func (d *Device) SetConditions(conditions []DeviceCondition) {
	d.Status.Conditions = conditions
}

func (d *Device) GetConditions() []DeviceCondition {
	return d.Status.Conditions
}

func (d *Device) IsAddedToEdgeX() bool {
	return d.Status.Synced
}

//+kubebuilder:object:root=true

// DeviceList contains a list of Device
type DeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Device `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Device{}, &DeviceList{})
}
