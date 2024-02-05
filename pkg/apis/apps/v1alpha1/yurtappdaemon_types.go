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

// YurtAppDaemonConditionType indicates valid conditions type of a YurtAppDaemon.
type YurtAppDaemonConditionType string

const (
	// WorkLoadProvisioned means all the expected workload are provisioned
	WorkLoadProvisioned YurtAppDaemonConditionType = "WorkLoadProvisioned"
	// WorkLoadUpdated means all the workload are updated.
	WorkLoadUpdated YurtAppDaemonConditionType = "WorkLoadUpdated"
	// WorkLoadFailure is added to a YurtAppSet when one of its workload has failure during its own reconciling.
	WorkLoadFailure YurtAppDaemonConditionType = "WorkLoadFailure"
)

// YurtAppDaemonSpec defines the desired state of YurtAppDaemon
type YurtAppDaemonSpec struct {
	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// WorkloadTemplate describes the pool that will be created.
	// +optional
	WorkloadTemplate WorkloadTemplate `json:"workloadTemplate,omitempty"`

	// NodePoolSelector is a label query over nodepool that should match the replica count.
	// It must match the nodepool's labels.
	NodePoolSelector *metav1.LabelSelector `json:"nodepoolSelector"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// YurtAppDaemonStatus defines the observed state of YurtAppDaemon
type YurtAppDaemonStatus struct {
	// ObservedGeneration is the most recent generation observed for this YurtAppDaemon. It corresponds to the
	// YurtAppDaemon's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the YurtAppDaemon. The YurtAppDaemon controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the YurtAppDaemon.
	CurrentRevision string `json:"currentRevision"`

	// Represents the latest available observations of a YurtAppDaemon's current state.
	// +optional
	Conditions []YurtAppDaemonCondition `json:"conditions,omitempty"`

	OverriderRef string `json:"overriderRef,omitempty"`

	// Records the topology detailed information of each workload.
	// +optional
	WorkloadSummaries []WorkloadSummary `json:"workloadSummary,omitempty"`

	// TemplateType indicates the type of PoolTemplate
	TemplateType TemplateType `json:"templateType"`

	// NodePools indicates the list of node pools selected by YurtAppDaemon
	NodePools []string `json:"nodepools,omitempty"`
}

// YurtAppDaemonCondition describes current state of a YurtAppDaemon.
type YurtAppDaemonCondition struct {
	// Type of in place set condition.
	Type YurtAppDaemonConditionType `json:"type,omitempty"`

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
// +kubebuilder:resource:scope=Namespaced,path=yurtappdaemons,shortName=yad,categories=all
// +kubebuilder:printcolumn:name="WorkloadTemplate",type="string",JSONPath=".status.templateType",description="The WorkloadTemplate Type."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="OverriderRef",type="string",JSONPath=".status.overriderRef",description="The name of overrider bound to this yurtappdaemon"
// +kubebuilder:deprecatedversion:warning="apps.openyurt.io/v1alpha1 YurtAppDaemon is deprecated; use apps.openyurt.io/v1beta1 YurtAppSet;"

// YurtAppDaemon is the Schema for the samples API
type YurtAppDaemon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtAppDaemonSpec   `json:"spec,omitempty"`
	Status YurtAppDaemonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// YurtAppDaemonList contains a list of YurtAppDaemon
type YurtAppDaemonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtAppDaemon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtAppDaemon{}, &YurtAppDaemonList{})
}
