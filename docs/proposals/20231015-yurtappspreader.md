---
title: Proposal about YurtAppSpreader
authors:
  - "@vie-serendipity"
  - "@rambohe-ch"
reviewers:
  - ""
creation-date:
last-updated:
status:
---
# Proposal for YurtAppSpreader
- [Proposal for YurtAppSpreader](#proposal-for-yurtappspreader)
	- [Glossary](#glossary)
		- [YurtAppSpreader](#yurtappspreader)
	- [Summary](#summary)
	- [Motivation](#motivation)
		- [Goals](#goals)
		- [Non-Goals/Future Work](#non-goalsfuture-work)
	- [Proposal](#proposal)
	- [Implementation History](#implementation-history)

## Glossary
### YurtAppSpreader
YurtAppSpreader is a new CRD in place of YurtAppSet/YurtAppDaemon. It provides affinity to assign pods to nodes. Node affinity functions like the nodeSelector field but is more expressive and allows you to specify soft rules. 
## Summary
Due to the multi-region nature of openyurt, we have introduced yurtappset and yurtappdaemon for 
workload distribution and management. While they serve different scenarios and functionalities, 
it is evident that their functionalities overlap significantly, making it feasible to consolidate 
them into a single component through an upgrade. Considering the existing user base and the introduction 
of the multi-region rendering engine, yurtappoverrider, the ability to configure multiple regions in yurtappset 
and yurtappdaemon is unnecessary. Therefore, introducing a new component seems 
more reasonable, as it can encompass the capabilities of both components, allowing users to gradually 
migrate from yurtappset and yurtappdaemon to the new component. Furthermore, the new component will 
offer enhanced expressiveness and the ability to define soft rules through affinity.
## Motivation
Considering that the existing yurtappdaemon and yurtappset have similar functionality and are both used for 
workload distribution, their abilities could be integrated into one component. In addition, k8s explicitly
specify affinity is more expressive than selector. Replacing nodeselector with affinity would be more appropriate.
### Goals
1. Add affinity
2. Implement the abilities of yurtappset and yurtappdaemon
### Non-Goals/Future Work
1. Deprecate YurtAppSet/YurtAppDaemon
2. Completion of migration from YurtAppSet/YurtAppDaemon to YurtAppSpreader
## Proposal
Yurtappspreader retains most of the fields from yurtappset and yurtappdaemon, 
except that it replaces nodeselector with affinity and removes the 
previously multi-node pool configuration capability, which is now handled by yurtappoverrider.
```go
type TemplateType string

const (
	StatefulSetTemplateType TemplateType = "StatefulSet"
	DeploymentTemplateType  TemplateType = "Deployment"
)

// YurtAppSpreaderConditionType indicates valid conditions type of a YurtAppSpreader.
type YurtAppSpreaderConditionType string

const (
	// PoolProvisioned means all the expected pools are provisioned and unexpected pools are deleted.
	PoolProvisioned YurtAppSpreaderConditionType = "PoolProvisioned"
	// PoolUpdated means all the pools are updated.
	PoolUpdated YurtAppSpreaderConditionType = "PoolUpdated"
	// PoolFailure is added to a YurtAppSpreader when one of its pools has failure during its own reconciling.
	PoolFailure YurtAppSpreaderConditionType = "PoolFailure"
)

// YurtAppSpreaderSpec defines the desired state of YurtAppSpreader.
type YurtAppSpreaderSpec struct {
	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// WorkloadTemplate describes the pool that will be created.
	// +optional
	WorkloadTemplate WorkloadTemplate `json:"workloadTemplate,omitempty"`

	// Affinity constrain which nodes Pod can be scheduled on based on node labels
	Affinity *corev1.Affinity `json:"affinity"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// WorkloadTemplate defines the pool template under the YurtAppSpreader.
// YurtAppSpreader will provision every pool based on one workload templates in WorkloadTemplate.
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

// YurtAppSpreaderStatus defines the observed state of YurtAppSpreader.
type YurtAppSpreaderStatus struct {
	// ObservedGeneration is the most recent generation observed for this YurtAppSpreader. It corresponds to the
	// YurtAppSpreader's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the YurtAppSpreader. The YurtAppSpreader controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the YurtAppSpreader.
	CurrentRevision string `json:"currentRevision"`

	// Represents the latest available observations of a YurtAppSpreader's current state.
	// +optional
	Conditions []YurtAppSpreaderCondition `json:"conditions,omitempty"`

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

// YurtAppSpreaderCondition describes current state of a YurtAppSpreader.
type YurtAppSpreaderCondition struct {
	// Type of in place set condition.
	Type YurtAppSpreaderConditionType `json:"type,omitempty"`

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
// +kubebuilder:resource:shortName=yas
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="WorkloadTemplate",type="string",JSONPath=".status.templateType",description="The WorkloadTemplate Type."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="OverriderRef",type="string",JSONPath=".status.overriderRef",description="The name of overrider bound to this YurtAppSpreader"

// YurtAppSpreader is the Schema for the YurtAppSpreaders API
type YurtAppSpreader struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtAppSpreaderSpec   `json:"spec,omitempty"`
	Status YurtAppSpreaderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// YurtAppSpreaderList contains a list of YurtAppSpreader
type YurtAppSpreaderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtAppSpreader `json:"items"`
}

```

## Implementation History

- [ ] Design CRD of YurtAppSpreader
- [ ] Present proposal
- [ ] First round of feedback from community
- [ ] Implement controllers and webhooks of YurtAppSpreader
