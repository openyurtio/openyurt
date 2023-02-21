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
	// Type of Static Pod upgrade. Can be "auto" or "ota".
	Type StaticPodUpgradeStrategyType `json:"type,omitempty"`

	// Auto upgrade config params. Present only if type = "auto".
	//+optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// StaticPodUpgradeStrategyType is a strategy according to which a static pod gets upgraded.
type StaticPodUpgradeStrategyType string

const (
	AutoStaticPodUpgradeStrategyType StaticPodUpgradeStrategyType = "auto"
	OTAStaticPodUpgradeStrategyType  StaticPodUpgradeStrategyType = "ota"
)

// StaticPodSpec defines the desired state of StaticPod
type StaticPodSpec struct {
	// StaticPodName indicates the static pod desired to be upgraded.
	StaticPodName string `json:"staticPodName"`

	// StaticPodManifest indicates the Static Pod desired to be upgraded. The corresponding
	// manifest file name is `StaticPodManifest.yaml`.
	// +optional
	StaticPodManifest string `json:"staticPodManifest,omitempty"`

	// Namespace indicates the namespace of target static pod
	// +optional
	StaticPodNamespace string `json:"staticPodNamespace,omitempty"`

	// An upgrade strategy to replace existing static pods with new ones.
	UpgradeStrategy StaticPodUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// An object that describes the desired upgrade static pod.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template corev1.PodTemplateSpec `json:"template,omitempty"`
}

type StaticPodConditionType string

const (
	// StaticPodUpgradeSuccess means static pods on all nodes have been upgraded to the latest version
	StaticPodUpgradeSuccess StaticPodConditionType = "UpgradeSuccess"

	// StaticPodUpgradeExecuting means static pods upgrade task is in progress
	StaticPodUpgradeExecuting StaticPodConditionType = "Upgrading"

	// StaticPodUpgradeFailed means that exist pods failed to upgrade during the upgrade process
	StaticPodUpgradeFailed StaticPodConditionType = "UpgradeFailed"
)

// StaticPodCondition describes the state of a StaticPodCondition at a certain point.
type StaticPodCondition struct {
	// Type of StaticPod condition.
	Type StaticPodConditionType `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human-readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// StaticPodStatus defines the observed state of StaticPod
type StaticPodStatus struct {
	// The total number of static pods
	TotalNumber int32 `json:"totalNumber"`

	// The number of static pods that should be upgraded.
	DesiredNumber int32 `json:"desiredNumber"`

	// The number of static pods that have been upgraded.
	UpgradedNumber int32 `json:"upgradedNumber"`

	// Represents the latest available observations of StaticPod's current state.
	// +optional
	Conditions []StaticPodCondition `json:"conditions"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=staticpods,shortName=sp,categories=all
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
//+kubebuilder:printcolumn:name="TotalNumber",type="integer",JSONPath=".status.totalNumber",description="The total number of static pods"
//+kubebuilder:printcolumn:name="DesiredNumber",type="integer",JSONPath=".status.desiredNumber",description="The number of static pods that desired to be upgraded"
//+kubebuilder:printcolumn:name="UpgradedNumber",type="integer",JSONPath=".status.upgradedNumber",description="The number of static pods that have been upgraded"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].type"

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
