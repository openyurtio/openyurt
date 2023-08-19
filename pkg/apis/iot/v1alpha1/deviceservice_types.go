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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeviceServiceFinalizer = "iot.openyurt.io/deviceservice"
)

// DeviceServiceConditionType indicates valid conditions type of a Device Service.
type DeviceServiceConditionType string

// DeviceServiceSpec defines the desired state of DeviceService
type DeviceServiceSpec struct {
	BaseAddress string `json:"baseAddress"`
	// Information describing the device
	Description string `json:"description,omitempty"`
	// tags or other labels applied to the device service for search or other
	// identification needs on the EdgeX Foundry
	Labels []string `json:"labels,omitempty"`
	// Device Service Admin State
	AdminState AdminState `json:"adminState,omitempty"`
	// True means deviceService is managed by cloud, cloud can update the related fields
	// False means cloud can't update the fields
	Managed bool `json:"managed,omitempty"`
	// NodePool indicates which nodePool the deviceService comes from
	NodePool string `json:"nodePool,omitempty"`
}

// DeviceServiceStatus defines the observed state of DeviceService
type DeviceServiceStatus struct {
	// Synced indicates whether the device already exists on both OpenYurt and edge platform
	Synced bool `json:"synced,omitempty"`
	// the Id assigned by the edge platform
	EdgeId string `json:"edgeId,omitempty"`
	// time in milliseconds that the device last reported data to the core
	LastConnected int64 `json:"lastConnected,omitempty"`
	// time in milliseconds that the device last reported data to the core
	LastReported int64 `json:"lastReported,omitempty"`
	// Device Service Admin State
	AdminState AdminState `json:"adminState,omitempty"`
	// current deviceService state
	// +optional
	Conditions []DeviceServiceCondition `json:"conditions,omitempty"`
}

// DeviceServiceCondition describes current state of a Device.
type DeviceServiceCondition struct {
	// Type of in place set condition.
	Type DeviceServiceConditionType `json:"type,omitempty"`

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
// +kubebuilder:resource:shortName=dsvc
// +kubebuilder:printcolumn:name="NODEPOOL",type="string",JSONPath=".spec.nodePool",description="The nodepool of deviceService"
// +kubebuilder:printcolumn:name="SYNCED",type="boolean",JSONPath=".status.synced",description="The synced status of deviceService"
// +kubebuilder:printcolumn:name="MANAGED",type="boolean",priority=1,JSONPath=".spec.managed",description="The managed status of deviceService"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// DeviceService is the Schema for the deviceservices API
type DeviceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceServiceSpec   `json:"spec,omitempty"`
	Status DeviceServiceStatus `json:"status,omitempty"`
}

func (ds *DeviceService) SetConditions(conditions []DeviceServiceCondition) {
	ds.Status.Conditions = conditions
}

func (ds *DeviceService) GetConditions() []DeviceServiceCondition {
	return ds.Status.Conditions
}

//+kubebuilder:object:root=true

// DeviceServiceList contains a list of DeviceService
type DeviceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceService{}, &DeviceServiceList{})
}
