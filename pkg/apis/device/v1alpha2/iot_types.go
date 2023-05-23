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

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// name of finalizer
	IoTFinalizer = "device.openyurt.io"

	LabelIoTGenerate = "device.openyurt.io/generate"
)

// IoT platform supported by openyurt
const (
	IoTPlatformEdgeX = "edgex"
)

// IoTConditionType indicates valid conditions type of a IoT.
type IoTConditionType string
type IoTConditionSeverity string

// Component defines the components of EdgeX
type Component struct {
	Name string `json:"name"`

	// +optional
	Image string `json:"image,omitempty"`
}

// IoTSpec defines the desired state of IoT
type IoTSpec struct {
	Version string `json:"version,omitempty"`

	ImageRegistry string `json:"imageRegistry,omitempty"`

	PoolName string `json:"poolName,omitempty"`

	// +optional
	Platform string `json:"platform,omitempty"`

	// +optional
	Components []Component `json:"components,omitempty"`

	// +optional
	Security bool `json:"security,omitempty"`
}

// IoTStatus defines the observed state of IoT
type IoTStatus struct {
	// +optional
	Ready bool `json:"ready,omitempty"`

	// +optional
	Initialized bool `json:"initialized,omitempty"`

	// +optional
	ReadyComponentNum int32 `json:"readyComponentNum,omitempty"`

	// +optional
	UnreadyComponentNum int32 `json:"unreadyComponentNum,omitempty"`

	// Current Edgex state
	// +optional
	Conditions []IoTCondition `json:"conditions,omitempty"`
}

// IoTCondition describes current state of a IoT.
type IoTCondition struct {
	// Type of in place set condition.
	Type IoTConditionType `json:"type,omitempty"`

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
// +kubebuilder:resource:scope=Namespaced,path=iots,shortName=iot,categories=all
// +kubebuilder:printcolumn:name="READY",type="boolean",JSONPath=".status.ready",description="The edgex ready status"
// +kubebuilder:printcolumn:name="ReadyComponentNum",type="integer",JSONPath=".status.readyComponentNum",description="The Ready Component."
// +kubebuilder:printcolumn:name="UnreadyComponentNum",type="integer",JSONPath=".status.unreadyComponentNum",description="The Unready Component."
// +kubebuilder:storageversion

// IoT is the Schema for the samples API
type IoT struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IoTSpec   `json:"spec,omitempty"`
	Status IoTStatus `json:"status,omitempty"`
}

func (c *IoT) GetConditions() []IoTCondition {
	return c.Status.Conditions
}

func (c *IoT) SetConditions(conditions []IoTCondition) {
	c.Status.Conditions = conditions
}

//+kubebuilder:object:root=true

// IoTList contains a list of IoT
type IoTList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IoT `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IoT{}, &IoTList{})
}
