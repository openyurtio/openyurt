---
title: Proposal about nodepool and uniteddeployment
authors:
  - "@kadisi"
reviewers:
  - "@huangyuqi"
  - "@Fei-Guo"
  - "@charleszheng44"
creation-date: 2020-12-11
last-updated: 2020-12-11
status: implementable
---

- [Proposal about nodepool and uniteddeployment](#proposal-about-nodepool-and-uniteddeployment)
  - [Glossary](#glossary)
    - [NodePool](#NodePool)
    - [UnitedDeployment](#UnitedDeployment)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
  - [Proposal](#proposal)
    - [NodePool API](#nodepool-api)
    - [UnitedDeployment API](#uniteddeployment-api)
  - [Implementation History](#implementation-history)

# Proposal about NodePool and UnitedDeployment

## Glossary

### NodePool:
- NodePool is a new CRD resource used to represent a group of nodes. Nodes under the NodePool have the same attributes, such as geography, operating system, CPU architecture, and so on.

### UnitedDeployment:
- UnitedDeployment provides a new way to manage pods in multi-nodepools by using multiple workloads. it provides an alternative to achieve high availability in a cluster that consists of multiple nodepool - that is, managing multiple homogeneous workloads, and each workload is dedicated to a single pool. Pod distribution across nodepools is determined by the replica number of each workload. Since each Pool is associated with a workload, UnitedDeployment can support finer-grained rollout and deployment strategies.
- UnitedDeployment is a new CRD resource that makes sure the right number of the right kind of Pod are running and match the state you specified on a specific NodePool. This means all the pods of the workload can only be deployed and managed on the nodes within the same NodePool. Since the deployment of the workload pods is restrained on the same NodePool, UnitedDeployment can support finer-grained rollout and deployment strategies basing on the same attributes of the nodes like the same location.

## Summary

In the edge scenario, user intend to deploy the workloads to the computing nodes close to the consumers. There may only be one computing node in the same physical location, and there may also be multiple computing nodes in the same physical location. Therefore, from the computing node resource perspective of edge nodes, they need to be divided into different NodePool to represent the same set of features. After dividing nodes pool, there will be a demand for application of grouping management, users need to be deployed application according to the nodepool, combined with the concept of nodepool, to deploy applications on different nodes in the pool, pool dimensions at the nodes to expansion of application, upgrade, such as operation, at the same time, network access is also carried out in accordance with the node pool dimensions of network communication.

## Motivation

### NodePool
- Users can quickly learn which node pools or regions are in their cluster. You can also quickly see which nodes are in the node pool.
- User can uniformly type label, annotation, and TAINts on nodes under the node pool.
- This simplifies the operation and maintenance management of nodes.
- The nodes in the NodePool are ideally accessible to each other through the Intranet.
 
### UnitedDeployment
- User can use a template to deploy the application in a different node pool. This template can support one of Kubernetes deployment, Daemonset, Statefulset.
 
### Goals
- Define the API of NodePool
- Define the API of UnitedDeployment
- Provide yurtunit-manager controller

## Proposal

- What is the plan for implementing this feature?

  Provide yurtunit-manager operator to manage NodePool and UnitedDeployment CRD

### NodePool API

``` go

type NodePoolType string

const (
        Edge  NodePoolType = "Edge"
        Cloud NodePoolType = "Cloud"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
        // The type of the nodepool
        // +optional
        Type NodePoolType `json:"type,omitempty"`

        // A label query over nodes to consider for adding to the pool
        // +optional
        Selector *metav1.LabelSelector `json:"selector,omitempty"`

        // If specified, the Labels will be added to all nodes.
        // NOTE: existing labels with samy keys on the nodes will be overwritten.
        // +optional
        Labels map[string]string `json:"labels,omitempty"`

        // If specified, the Annotations will be added to all nodes.
        // NOTE: existing labels with samy keys on the nodes will be overwritten.
        // +optional
        Annotations map[string]string `json:"annotations,omitempty"`

        // If specified, the Taints will be added to all nodes.
        // +optional
        Taints []v1.Taint `json:"taints,omitempty"`
}

// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
        // Total number of ready nodes in the pool.
        // +optional
        ReadyNode int32 `json:"readyNode"`

        // Total number of not ready nodes in the pool.
        // +optional
        NotReadyNode int32 `json:"notReadyNode"`

        // The list of nodes' names in the pool
        // +optional
        Nodes []string `json:"nodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=nodepools,shortName=np,categories=all
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The type of nodepool"
// +kubebuilder:printcolumn:name="ReadyNodes",type="integer",JSONPath=".status.readyNode",description="The number of ready nodes in the pool"
// +kubebuilder:printcolumn:name="NotReadyNodes",type="integer",JSONPath=".status.notReadyNode"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +genclient:nonNamespaced

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// NodePool is the Schema for the nodepools API
type NodePool struct {
        metav1.TypeMeta   `json:",inline"`
        metav1.ObjectMeta `json:"metadata,omitempty"`

        Spec   NodePoolSpec   `json:"spec,omitempty"`
        Status NodePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolList contains a list of NodePool
type NodePoolList struct {
        metav1.TypeMeta `json:",inline"`
        metav1.ListMeta `json:"metadata,omitempty"`
        Items           []NodePool `json:"items"`
}

```

### UnitedDeployment API

``` go


type TemplateType string

const (
	StatefulSetTemplateType TemplateType = "StatefulSet"
	DeploymentTemplateType  TemplateType = "Deployment"
)

// UnitedDeploymentConditionType indicates valid condition type of a UnitedDeployment.
type UnitedDeploymentConditionType string

const (
	// PoolProvisioned means all the expected pools are provisioned and unexpected pools are deleted.
	PoolProvisioned UnitedDeploymentConditionType = "PoolProvisioned"
	// PoolUpdated means all the pools are updated.
	PoolUpdated UnitedDeploymentConditionType = "PoolUpdated"
	// PoolFailure is added to a UnitedDeployment when one of its pools has failure during its own reconciling.
	PoolFailure UnitedDeploymentConditionType = "PoolFailure"
)

// UnitedDeploymentSpec defines the desired state of UnitedDeployment.
type UnitedDeploymentSpec struct {
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

// Topology defines the spread detail of each pool under UnitedDeployment.
// A UnitedDeployment manages multiple homogeneous workloads which are called pool.
// Each of pools under the UnitedDeployment is described in Topology.
type Topology struct {
	// Contains the details of each pool. Each element in this array represents one pool
	// which will be provisioned and managed by UnitedDeployment.
	// +optional
	Pools []Pool `json:"pools,omitempty"`
}

// Pool defines the detail of a pool.
type Pool struct {
	// Indicates pool name as a DNS_LABEL, which will be used to generate
	// pool workload name prefix in the format '<deployment-name>-<pool-name>-'.
	// Name should be unique between all of the pools under one UnitedDeployment.
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
}

// UnitedDeploymentStatus defines the observed state of UnitedDeployment.
type UnitedDeploymentStatus struct {
	// ObservedGeneration is the most recent generation observed for this UnitedDeployment. It corresponds to the
	// UnitedDeployment's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the UnitedDeployment. The UnitedDeployment controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the UnitedDeployment.
	CurrentRevision string `json:"currentRevision"`

	// Represents the latest available observations of a UnitedDeployment's current state.
	// +optional
	Conditions []UnitedDeploymentCondition `json:"conditions,omitempty"`

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

// UnitedDeploymentCondition describes current state of a UnitedDeployment.
type UnitedDeploymentCondition struct {
	// Type of in place set condition.
	Type UnitedDeploymentConditionType `json:"type,omitempty"`

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
// +kubebuilder:resource:shortName=ud
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="TemplateType",type="string",JSONPath=".status.templateType",description="The WorkloadTemplate Type."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// UnitedDeployment is the Schema for the uniteddeployments API
type UnitedDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnitedDeploymentSpec   `json:"spec,omitempty"`
	Status UnitedDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UnitedDeploymentList contains a list of UnitedDeployment
type UnitedDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UnitedDeployment `json:"items"`
}

```

## Implementation History

+ [ ] 12/13/2020: nodepool and uniteddeployment crd
+ [ ] 12/15/2020: yurtunit-manager controller
+ [ ] 12/20/2020: yurtunit-manager release

