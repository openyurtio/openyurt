---
title: Proposal about uniteddaemonset
authors:
  - "@kadisi"
reviewers:
  - "@huangyuqi"
  - "@Fei-Guo"
creation-date: 2021-07-29
status: provisional 
---


# Proposal about UnitedDaemonSet

## Glossary

### UnitedDaemonSet:

A UnitedDaemonSet ensures that all (or some) NodePools run a copy of a Deployment or StatefulSet. As nodepools are added to the cluster, Deployment or Statefulset are added to them. As nodepools are removed from the cluster, those Deployments or Statefulset are garbage collected. Deleting a UnitedDaemonSet will clean up the Deployments or StatefulSet it created. The behavior of UnitedDaemonSet is similar to that of Daemonset, except that UnitedDaemonSet creates resources from a node pool.

  Some typical uses of a UnitedDaemonSet are:
  
    - running a deployment on every nodepool
    - running a statefulset on every nodepool

## Summary

In the edge scenario, compute nodes in different regions are assigned to the same node pool, where necessary system components such as CoreDNS, ingree-Controller, and so on are often deployed. We expect these necessary system components to be created automatically with the creation of the node pool. No additional human intervention is required.

## Motivation

### UnitedDaemonSet
- Users can automatically deploy system components to different node pools (with specific labels) using a template that supports Deployment and Statefulset.
 
### Goals
- Define the API of UnitedDaemonSet 
- Provide UnitedDaemonSet controller
- Provide UnitedDaemonSet webhook 

## Proposal

- What is the plan for implementing this feature?

  Provide yurtunit-manager operator to UnitedDaemonSet CRD

### UnitedDaemonSet API

``` go

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UnitedDaemonSetConditionType indicates valid conditions type of a UnitedDaemonSet.
type UnitedDaemonSetConditionType string

const (
	// WorkLoadProvisioned means all the expected workload are provisioned
	WorkLoadProvisioned UnitedDaemonSetConditionType = "WorkLoadProvisioned"
	// WorkLoadUpdated means all the workload are updated.
	WorkLoadUpdated UnitedDaemonSetConditionType = "WorkLoadUpdated"
	// WorkLoadFailure is added to a UnitedDeployment when one of its workload has failure during its own reconciling.
	WorkLoadFailure UnitedDaemonSetConditionType = "WorkLoadFailure"
)

// UnitedDaemonSetSpec defines the desired state of UnitedDaemonSet.
type UnitedDaemonSetSpec struct {
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

// UnitedDaemonSetStatus defines the observed state of UnitedDaemonSet.
type UnitedDaemonSetStatus struct {
	// ObservedGeneration is the most recent generation observed for this UnitedDaemonSet. It corresponds to the
	// UnitedDaemonSet's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the UnitedDaemonSet. The UnitedDaemonSet controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the UnitedDaemonSet.
	CurrentRevision string `json:"currentRevision"`

	// Represents the latest available observations of a UnitedDaemonSet's current state.
	// +optional
	Conditions []UnitedDaemonSetCondition `json:"conditions,omitempty"`

	// TemplateType indicates the type of PoolTemplate
	TemplateType TemplateType `json:"templateType"`

	// NodePools indicates the list of node pools selected by UnitedDaemonSet
	NodePools []string `json:"nodepools,omitempty"`
}

// UnitedDaemonSetCondition describes current state of a UnitedDaemonSet.
type UnitedDaemonSetCondition struct {
	// Type of in place set condition.
	Type UnitedDaemonSetConditionType `json:"type,omitempty"`

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

// UnitedDaemonSet is the Schema for the uniteddeployments API
type UnitedDaemonSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnitedDaemonSetSpec   `json:"spec,omitempty"`
	Status UnitedDaemonSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UnitedDaemonSetList contains a list of UnitedDaemonSet
type UnitedDaemonSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UnitedDaemonSet `json:"items"`
}

// WorkloadTemplate defines the pool template under the UnitedDeployment.
// UnitedDeployment will provision every pool based on one workload templates in WorkloadTemplate.
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

+ [ ] : uniteddaemonset api crd
+ [ ] : yurtunit-manager uniteddaemonset controller
+ [ ] : yurtunit-manager uniteddaemonset release

