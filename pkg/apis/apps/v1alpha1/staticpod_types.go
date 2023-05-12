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

// StaticPodUpgradeStrategy defines a strategy to upgrade a static pod.
type StaticPodUpgradeStrategy struct {
	// Type of Static Pod upgrade. Can be "AdvancedRollingUpdate" or "OTA".
	Type StaticPodUpgradeStrategyType `json:"type,omitempty"`

	// AdvancedRollingUpdate upgrade config params. Present only if type = "AdvancedRollingUpdate".
	//+optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// StaticPodUpgradeStrategyType is a strategy according to which a static pod gets upgraded.
type StaticPodUpgradeStrategyType string

const (
	AdvancedRollingUpdateStaticPodUpgradeStrategyType StaticPodUpgradeStrategyType = "AdvancedRollingUpdate"
	AutoStaticPodUpgradeStrategyType                  StaticPodUpgradeStrategyType = "auto"
	OTAStaticPodUpgradeStrategyType                   StaticPodUpgradeStrategyType = "OTA"
)

// StaticPodSpec defines the desired state of StaticPod
type StaticPodSpec struct {
	// StaticPodManifest indicates the Static Pod desired to be upgraded. The corresponding
	// manifest file name is `StaticPodManifest.yaml`.
	StaticPodManifest string `json:"staticPodManifest,omitempty"`

	// An upgrade strategy to replace existing static pods with new ones.
	UpgradeStrategy StaticPodUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// The number of old history to retain to allow rollback.
	// Defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// An object that describes the desired upgrade static pod.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template corev1.PodTemplateSpec `json:"template,omitempty"`
}

// StaticPodStatus defines the observed state of StaticPod
type StaticPodStatus struct {
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
// +kubebuilder:resource:shortName=sp
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
//+kubebuilder:printcolumn:name="TotalNumber",type="integer",JSONPath=".status.totalNumber",description="The total number of static pods"
//+kubebuilder:printcolumn:name="ReadyNumber",type="integer",JSONPath=".status.readyNumber",description="The number of ready static pods"
//+kubebuilder:printcolumn:name="UpgradedNumber",type="integer",JSONPath=".status.upgradedNumber",description="The number of static pods that have been upgraded"

// StaticPod is the Schema for the staticpods API
type StaticPod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaticPodSpec   `json:"spec,omitempty"`
	Status StaticPodStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StaticPodList contains a list of StaticPod
type StaticPodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticPod `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticPod{}, &StaticPodList{})
}
