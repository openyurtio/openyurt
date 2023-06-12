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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// YurtStaticSetUpgradeStrategy defines a strategy to upgrade static pods.
type YurtStaticSetUpgradeStrategy struct {
	// Type of YurtStaticSet upgrade. Can be "AdvancedRollingUpdate" or "OTA".
	Type YurtStaticSetUpgradeStrategyType `json:"type,omitempty"`

	// AdvancedRollingUpdate upgrade config params. Present only if type = "AdvancedRollingUpdate".
	//+optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// YurtStaticSetUpgradeStrategyType is a strategy according to which static pods gets upgraded.
type YurtStaticSetUpgradeStrategyType string

const (
	AdvancedRollingUpdateUpgradeStrategyType YurtStaticSetUpgradeStrategyType = "AdvancedRollingUpdate"
	OTAUpgradeStrategyType                   YurtStaticSetUpgradeStrategyType = "OTA"
)

// YurtStaticSetSpec defines the desired state of YurtStaticSet
type YurtStaticSetSpec struct {
	// StaticPodManifest indicates the file name of static pod manifest.
	// The corresponding manifest file name is `StaticPodManifest.yaml`.
	StaticPodManifest string `json:"staticPodManifest,omitempty"`

	// An upgrade strategy to replace existing static pods with new ones.
	UpgradeStrategy YurtStaticSetUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// The number of old history to retain to allow rollback.
	// Defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// An object that describes the desired spec of static pod.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template corev1.PodTemplateSpec `json:"template,omitempty"`
}

// YurtStaticSetStatus defines the observed state of YurtStaticSet
type YurtStaticSetStatus struct {
	// The total number of nodes that are running the static pod.
	TotalNumber int32 `json:"totalNumber"`

	// The number of ready static pods.
	ReadyNumber int32 `json:"readyNumber"`

	// The number of nodes that are running updated static pod.
	UpgradedNumber int32 `json:"upgradedNumber"`

	// The most recent generation observed by the static pod controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=yss
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
//+kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.totalNumber",description="The total number of static pods"
//+kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyNumber",description="The number of ready static pods"
//+kubebuilder:printcolumn:name="Upgraded",type="integer",JSONPath=".status.upgradedNumber",description="The number of static pods that have been upgraded"

// YurtStaticSet is the Schema for the yurtstaticsets API
type YurtStaticSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtStaticSetSpec   `json:"spec,omitempty"`
	Status YurtStaticSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// YurtStaticSetList contains a list of YurtStaticSet
type YurtStaticSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtStaticSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtStaticSet{}, &YurtStaticSetList{})
}
