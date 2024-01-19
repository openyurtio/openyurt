/*
Copyright 2021 The OpenYurt Authors.
Copyright 2020 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

@CHANGELOG
OpenYurt Authors:
change UnitedDeployment API Definition
*/

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type TemplateType string

const (
	StatefulSetTemplateType TemplateType = "StatefulSet"
	DeploymentTemplateType  TemplateType = "Deployment"
)

// YurtAppSetConditionType indicates valid conditions type of a YurtAppSet.
type YurtAppSetConditionType string

const (
	// PoolProvisioned means all the expected pools are provisioned and unexpected pools are deleted.
	PoolProvisioned YurtAppSetConditionType = "PoolProvisioned"
	// PoolUpdated means all the pools are updated.
	PoolUpdated YurtAppSetConditionType = "PoolUpdated"
	// PoolFailure is added to a YurtAppSet when one of its pools has failure during its own reconciling.
	PoolFailure YurtAppSetConditionType = "PoolFailure"
)

// YurtAppSetSpec defines the desired state of YurtAppSet.
type YurtAppSetSpec struct {
	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// WorkloadTemplate describes the pool that will be created.
	// +optional
	WorkloadTemplate WorkloadTemplate `json:"workloadTemplate,omitempty"`

	// Topology describes the pods distribution detail between each of pools.
	// +optional
	Topology Topology `json:"topology,omitempty"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
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

// Topology defines the spread detail of each pool under YurtAppSet.
// A YurtAppSet manages multiple homogeneous workloads which are called pool.
// Each of pools under the YurtAppSet is described in Topology.
type Topology struct {
	// Contains the details of each pool. Each element in this array represents one pool
	// which will be provisioned and managed by YurtAppSet.
	// +optional
	Pools []Pool `json:"pools,omitempty"`
}

// Pool defines the detail of a pool.
type Pool struct {
	// Indicates pool name as a DNS_LABEL, which will be used to generate
	// pool workload name prefix in the format '<deployment-name>-<pool-name>-'.
	// Name should be unique between all of the pools under one YurtAppSet.
	// Name is NodePool Name
	Name string `json:"name"`

	// Indicates the node selector to form the pool. Depending on the node selector,
	// pods provisioned could be distributed across multiple groups of nodes.
	// A pool's nodeSelectorTerm is not allowed to be updated.
	// +optional
	NodeSelectorTerm corev1.NodeSelectorTerm `json:"nodeSelectorTerm,omitempty"`

	// Indicates the tolerations the pods under this pool have.
	// A pool's tolerations is not allowed to be updated.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Indicates the number of the pod to be created under this pool.
	// +required
	Replicas *int32 `json:"replicas,omitempty"`

	// Indicates the patch for the templateSpec
	// Now support strategic merge path :https://kubernetes.io/docs/tasks/manage-kubernetes-objects/update-api-object-kubectl-patch/#notes-on-the-strategic-merge-patch
	// Patch takes precedence over Replicas fields
	// If the Patch also modifies the Replicas, use the Replicas value in the Patch
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Patch *runtime.RawExtension `json:"patch,omitempty"`
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

	// Records the topology detailed information of each workload.
	// +optional
	WorkloadSummaries []WorkloadSummary `json:"workloadSummary,omitempty"`

	OverriderRef string `json:"overriderRef,omitempty"`

	// Records the topology detail information of the replicas of each pool.
	// +optional
	PoolReplicas map[string]int32 `json:"poolReplicas,omitempty"`

	// The number of ready replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`

	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`

	// TemplateType indicates the type of PoolTemplate
	TemplateType TemplateType `json:"templateType"`
}

type WorkloadSummary struct {
	AvailableCondition corev1.ConditionStatus `json:"availableCondition"`
	Replicas           int32                  `json:"replicas"`
	ReadyReplicas      int32                  `json:"readyReplicas"`
	WorkloadName       string                 `json:"workloadName"`
}

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
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="WorkloadTemplate",type="string",JSONPath=".status.templateType",description="The WorkloadTemplate Type."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="OverriderRef",type="string",JSONPath=".status.overriderRef",description="The name of overrider bound to this yurtappset"
// +kubebuilder:deprecatedversion:warning="apps.openyurt.io/v1alpha1 YurtAppSet is deprecated; use apps.openyurt.io/v1beta1 YurtAppSet; v1alpha1 YurtAppSet.Status.WorkloadSummary should not be used"

// YurtAppSet is the Schema for the yurtAppSets API
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
