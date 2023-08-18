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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeviceProfileFinalizer = "iot.openyurt.io/deviceprofile"
)

type DeviceResource struct {
	Description string             `json:"description"`
	Name        string             `json:"name"`
	Tag         string             `json:"tag,omitempty"`
	IsHidden    bool               `json:"isHidden"`
	Properties  ResourceProperties `json:"properties"`
	Attributes  map[string]string  `json:"attributes,omitempty"`
}

type ResourceProperties struct {
	ReadWrite    string `json:"readWrite,omitempty"`    // Read/Write Permissions set for this property
	Minimum      string `json:"minimum,omitempty"`      // Minimum value that can be get/set from this property
	Maximum      string `json:"maximum,omitempty"`      // Maximum value that can be get/set from this property
	DefaultValue string `json:"defaultValue,omitempty"` // Default value set to this property if no argument is passed
	Mask         string `json:"mask,omitempty"`         // Mask to be applied prior to get/set of property
	Shift        string `json:"shift,omitempty"`        // Shift to be applied after masking, prior to get/set of property
	Scale        string `json:"scale,omitempty"`        // Multiplicative factor to be applied after shifting, prior to get/set of property
	Offset       string `json:"offset,omitempty"`       // Additive factor to be applied after multiplying, prior to get/set of property
	Base         string `json:"base,omitempty"`         // Base for property to be applied to, leave 0 for no power operation (i.e. base ^ property: 2 ^ 10)
	Assertion    string `json:"assertion,omitempty"`
	MediaType    string `json:"mediaType,omitempty"`
	Units        string `json:"units,omitempty"`
	ValueType    string `json:"valueType,omitempty"`
}

type DeviceCommand struct {
	Name               string              `json:"name"`
	IsHidden           bool                `json:"isHidden"`
	ReadWrite          string              `json:"readWrite"`
	ResourceOperations []ResourceOperation `json:"resourceOperations"`
}

type ResourceOperation struct {
	DeviceResource string            `json:"deviceResource,omitempty"`
	Mappings       map[string]string `json:"mappings,omitempty"`
	DefaultValue   string            `json:"defaultValue"`
}

// DeviceProfileSpec defines the desired state of DeviceProfile
type DeviceProfileSpec struct {
	// NodePool specifies which nodePool the deviceProfile belongs to
	NodePool    string `json:"nodePool,omitempty"`
	Description string `json:"description,omitempty"`
	// Manufacturer of the device
	Manufacturer string `json:"manufacturer,omitempty"`
	// Model of the device
	Model string `json:"model,omitempty"`
	// Labels used to search for groups of profiles on EdgeX Foundry
	Labels          []string         `json:"labels,omitempty"`
	DeviceResources []DeviceResource `json:"deviceResources,omitempty"`
	DeviceCommands  []DeviceCommand  `json:"deviceCommands,omitempty"`
}

// DeviceProfileStatus defines the observed state of DeviceProfile
type DeviceProfileStatus struct {
	EdgeId string `json:"id,omitempty"`
	Synced bool   `json:"synced,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dp
// +kubebuilder:printcolumn:name="NODEPOOL",type="string",JSONPath=".spec.nodePool",description="The nodepool of deviceProfile"
// +kubebuilder:printcolumn:name="SYNCED",type="boolean",JSONPath=".status.synced",description="The synced status of deviceProfile"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// DeviceProfile represents the attributes and operational capabilities of a device.
// It is a template for which there can be multiple matching devices within a given system.
// NOTE This struct is derived from
// edgex/go-mod-core-contracts/models/deviceprofile.go
type DeviceProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceProfileSpec   `json:"spec,omitempty"`
	Status DeviceProfileStatus `json:"status,omitempty"`
}

func (dp *DeviceProfile) IsAddedToEdgeX() bool {
	return dp.Status.Synced
}

//+kubebuilder:object:root=true

// DeviceProfileList contains a list of DeviceProfile
type DeviceProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceProfile{}, &DeviceProfileList{})
}
