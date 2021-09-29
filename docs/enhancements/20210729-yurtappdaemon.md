---
title: Proposal about YurtAppDaemon
authors:
  - "@kadisi"
reviewers:
  - "@huangyuqi"
  - "@Fei-Guo"
creation-date: 2021-07-29
status: provisional 
---


# Proposal about YurtAppDaemon

## Glossary

### YurtAppDaemon:

A YurtAppDaemon ensures that all (or some) NodePools run a copy of a Deployment or StatefulSet. As nodepools are added to the cluster, Deployment or Statefulset are added to them. As nodepools are removed from the cluster, those Deployments or Statefulset are garbage collected. Deleting a YurtAppDaemon will clean up the Deployments or StatefulSet it created. The behavior of YurtAppDaemon is similar to that of Daemonset, except that YurtAppDaemon creates resources from a node pool.

  Some typical uses of a YurtAppDaemon are:
  
    - running a deployment on every nodepool
    - running a statefulset on every nodepool

## Summary

In the edge scenario, compute nodes in different regions are assigned to the same node pool, where necessary system components such as CoreDNS, ingree-Controller, and so on are often deployed. We expect these necessary system components to be created automatically with the creation of the node pool. No additional human intervention is required.

## Motivation

### YurtAppDaemon
- Users can automatically deploy system components to different node pools (with specific labels) using a template that supports Deployment and Statefulset.
 
### Goals
- Define the API of YurtAppDaemon 
- Provide YurtAppDaemon controller
- Provide YurtAppDaemon webhook 

## Proposal

- What is the plan for implementing this feature?

  Provide yurt-app-manager operator to YurtAppDaemon CRD

### YurtAppDaemon API

``` go

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
	// WorkLoadFailure is added to a YurtAppDaemonConditionType when one of its workload has failure during its own reconciling.
	WorkLoadFailure YurtAppDaemonConditionType = "WorkLoadFailure"
)

// YurtAppDaemonSpec defines the desired state of YurtAppDaemon.
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

// YurtAppDaemonStatus defines the observed state of YurtAppDaemon.
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
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=udd
// +kubebuilder:printcolumn:name="WorkloadTemplate",type="string",JSONPath=".status.templateType",description="The WorkloadTemplate Type."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// YurtAppDaemon is the Schema for the YurtAppDaemon API
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

// WorkloadTemplate defines the pool template under the YurtAppDaemon.
// YurtAppDaemon will provision every pool based on one workload templates in WorkloadTemplate.
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
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              appsv1.StatefulSetSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the pool template of Deployment.
type DeploymentTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              appsv1.DeploymentSpec `json:"spec"`
}


```

## Implementation History

+ [ ] : YurtAppDaemon api crd
+ [ ] : yurt-app-manager YurtAppDaemon controller
+ [ ] : yurt-app-manager YurtAppDaemon release

