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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageItem specifies the corresponding container and the claimed image
type ImageItem struct {
	// ContainerName represents name of the container
	// in which the Image will be replaced
	ContainerName string `json:"containerName"`
	// ImageClaim represents the claimed image name
	//which is injected into the container above
	ImageClaim string `json:"imageClaim"`
}

// Item represents configuration to be injected.
// Only one of its members may be specified.
type Item struct {
	// +optional
	Image *ImageItem `json:"image,omitempty"`
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

type Operation string

const (
	Default Operation = "default" // strategic merge patch
	ADD     Operation = "add"     // json patch
	REMOVE  Operation = "remove"  // json patch
	REPLACE Operation = "replace" // json patch
)

type Patch struct {
	// Path represents the path in the json patch
	Path string `json:"path"`
	// type represents the operation
	// default is strategic merge patch
	// +kubebuilder:validation:Enum=add;remove;replace
	Operation Operation `json:"operation"`
	// Indicates the patch for the template
	// +optional
	Value apiextensionsv1.JSON `json:"value,omitempty"`
}

// Describe detailed multi-region configuration of the subject
// Entry describe a set of nodepools and their shared or identical configurations
type Entry struct {
	Pools []string `json:"pools"`
	// +optional
	Items []Item `json:"items,omitempty"`
	// Convert Patch struct into json patch operation
	// +optional
	Patches []Patch `json:"patches,omitempty"`
}

// Describe the object Entries belongs
type Subject struct {
	metav1.TypeMeta `json:",inline"`
	// Name is the name of YurtAppSet or YurtAppDaemon
	Name string `json:"name"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=yacr
// +kubebuilder:printcolumn:name="Subject",type="string",JSONPath=".subject.kind",description="The subject kind of this overrider."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

type YurtAppOverrider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Subject           Subject `json:"subject"`
	Entries           []Entry `json:"entries"`
}

//+kubebuilder:object:root=true

// YurtAppOverriderList contains a list of YurtAppOverrider
type YurtAppOverriderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtAppOverrider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtAppOverrider{}, &YurtAppOverriderList{})
}
