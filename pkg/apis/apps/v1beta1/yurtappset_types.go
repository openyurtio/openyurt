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

package v1beta1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// YurtAppSetSpec defines the desired state of YurtAppSet.
type YurtAppSetSpec struct {
	// Workload defines the workload to be deployed in the nodepools
	Workload `json:"workload"`

	// NodePoolSelector is a label query over nodepool in which workloads should be deployed in.
	// It must match the nodepool's labels.
	// +optional
	NodePoolSelector *metav1.LabelSelector `json:"nodepoolSelector,omitempty"`

	// Pools is a list of selected nodepools specified with nodepool id in which workloads should be deployed in.
	// It is primarily used for compatibility with v1alpha1 version and NodePoolSelector should be preferred to choose nodepools
	// +optional
	Pools []string `json:"pools,omitempty"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// Workload defines the workload to be deployed in the nodepools
type Workload struct {
	// WorkloadTemplate defines the pool template under the YurtAppSet.
	WorkloadTemplate `json:"workloadTemplate"`
	// WorkloadTemplate defines the customization that will be applied to certain workloads in specified nodepools.
	// +optional
	WorkloadTweaks []WorkloadTweak `json:"workloadTweaks,omitempty"`
}

// WorkloadTemplate defines the pool template under the YurtAppSet.
// YurtAppSet will provision every pool based on one workload templates in WorkloadTemplate.
// WorkloadTemplate now support statefulset and deployment
// Only one of its members may be specified.
type WorkloadTemplate struct {
	// StatefulSet template
	// +optional
	StatefulSetTemplate *StatefulSetTemplateSpec `json:"statefulSetTemplate,omitempty"`

	// Deployment template
	// +optional
	DeploymentTemplate *DeploymentTemplateSpec `json:"deploymentTemplate,omitempty"`
}

// StatefulSetTemplateSpec defines the pool template of StatefulSet.
type StatefulSetTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec appsv1.StatefulSetSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the pool template of Deployment.
type DeploymentTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec appsv1.DeploymentSpec `json:"spec"`
}

// WorkloadTweak Describe detailed multi-region configuration of the subject
// BasicTweaks and AdvancedTweaks describe a set of nodepools and their shared or identical configurations
type WorkloadTweak struct {
	// NodePoolSelector is a label query over nodepool in which workloads should be adjusted.
	// +optional
	NodePoolSelector *metav1.LabelSelector `json:"nodepoolSelector,omitempty"`
	// Pools is a list of selected nodepools specified with nodepool id in which workloads should be adjusted.
	// Pools is not recommended and NodePoolSelector should be preferred
	// +optional
	Pools []string `json:"pools,omitempty"`
	// Tweaks is the adjustment can be applied to a certain workload in specified nodepools such as image and replicas
	Tweaks `json:"tweaks"`
}

// Tweaks represents configuration to be injected.
// Only one of its members may be specified.
type Tweaks struct {
	// +optional
	// Replicas overrides the replicas of the workload
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	// ContainerImages is a list of container images to be injected to a certain workload
	ContainerImages []ContainerImage `json:"containerImages,omitempty"`
	// +optional
	// Patches is a list of advanced tweaks to be applied to a certain workload
	// It can add/remove/replace the field values of specified paths in the template.
	Patches []Patch `json:"patches,omitempty"`
}

// ContainerImage specifies the corresponding container and the target image
type ContainerImage struct {
	// Name represents name of the container in which the Image will be replaced
	Name string `json:"name"`
	// TargetImage represents the image name which is injected into the container above
	TargetImage string `json:"targetImage"`
}

type Operation string

const (
	ADD     Operation = "add"     // json patch
	REMOVE  Operation = "remove"  // json patch
	REPLACE Operation = "replace" // json patch
)

type Patch struct {
	// Path represents the path in the json patch
	Path string `json:"path"`
	// Operation represents the operation
	// +kubebuilder:validation:Enum=add;remove;replace
	Operation Operation `json:"operation"`
	// Indicates the value of json patch
	// +optional
	Value apiextensionsv1.JSON `json:"value,omitempty"`
}

// YurtAppSetStatus defines the observed state of YurtAppSet.
type YurtAppSetStatus struct {
	// ObservedGeneration is the most recent generation observed for this YurtAppSet. It corresponds to the
	// YurtAppSet's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the YurtAppSet. The YurtAppSet controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the YurtAppSet.
	CurrentRevision string `json:"currentRevision"`

	// Represents the latest available observations of a YurtAppSet's current state.
	// +optional
	Conditions []YurtAppSetCondition `json:"conditions,omitempty"`

	// The number of ready workloads.
	ReadyWorkloads int32 `json:"readyWorkloads"`

	// The number of updated workloads.
	// +optional
	UpdatedWorkloads int32 `json:"updatedWorkloads"`

	// TotalWorkloads is the most recently observed number of workloads.
	TotalWorkloads int32 `json:"totalWorkloads"`
}

// YurtAppSetConditionType indicates valid conditions type of a YurtAppSet.
type YurtAppSetConditionType string

const (
	// AppDispatched means all the expected workloads are created successfully.
	AppSetAppDispatchced YurtAppSetConditionType = "AppDispatched"
	// AppDeleted means all the unexpected workloads are deleted successfully.
	AppSetAppDeleted YurtAppSetConditionType = "AppDeleted"
	// AppUpdated means all expected workloads are updated successfully.
	AppSetAppUpdated YurtAppSetConditionType = "AppUpdated"
	// AppUpdated means all workloads are ready
	AppSetAppReady YurtAppSetConditionType = "AppReady"
	// PoolFound is added to a YurtAppSet when all specified nodepools are found
	// if no nodepools meets the nodepoolselector or pools of yurtappset, PoolFound condition is set to false
	AppSetPoolFound YurtAppSetConditionType = "PoolFound"
)

// YurtAppSetCondition describes current state of a YurtAppSet.
type YurtAppSetCondition struct {
	// Type of in place set condition.
	Type YurtAppSetConditionType `json:"type,omitempty"`

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
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=yas,categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.totalWorkloads",description="The total number of workloads."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyWorkloads",description="The number of workloads ready."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedWorkloads",description="The number of workloads updated."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:storageversion

// YurtAppSet is the Schema for the YurtAppSets API
type YurtAppSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtAppSetSpec   `json:"spec,omitempty"`
	Status YurtAppSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// YurtAppSetList contains a list of YurtAppSet
type YurtAppSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtAppSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtAppSet{}, &YurtAppSetList{})
}
