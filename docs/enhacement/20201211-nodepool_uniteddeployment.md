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

   * [Proposal about nodepool and uniteddeployment](#proposal-about-nodepool-and-uniteddeployment)
      * [Glossary](#glossary)
      * [Summary](#summary)
      * [Motivation](#motivation)
         * [Goals](#goals)
      * [Proposal](#proposal)
         * [NodePool API](#nodepool-api)
         * [UnitedDeployment API](#uniteddeployment-api)
      * [Implementation History](#implementation-history)


# Proposal about nodepool and uniteddeployment  

## Glossary

NodePool:
    A node pool is a group of nodes within a same region in the edge computing scenario. 

UnitedDeployment:
    UnitedDeployment provides a new way to manage pods in multi-nodepools by using multiple workloads. it provides an alternative to achieve high availability in a cluster that consists of multiple nodepool - that is, managing multiple homogeneous workloads, and each workload is dedicated to a single Subset. Pod distribution across nodepools is determined by the replica number of each workload. Since each Subset is associated with a workload, UnitedDeployment can support finer-grained rollout and deployment strategies.
    
## Summary

In the edge scenario, compute nodes have strong regional attributes. There may only be one worker node in the same physical location, and there may also be multiple worker nodes in the same physical location. Therefore, from the work node resource perspective of edge nodes, they need to be divided into different node pools (NodePool) to represent the same set of features. After dividing nodes pool, there will be a demand for application of grouping management, users need to be deployed application according to the nodepool, combined with the concept of nodepool, to deploy applications on different nodes in the pool, pool dimensions at the nodes to expansion of application, upgrade, such as operation, at the same time, network access is also carried out in accordance with the node pool dimensions of network communication.

## Motivation

- NodePool
    Users can quickly learn which node pools or regions are in their cluster. You can also quickly see which nodes are in the node pool.
    User can uniformly type label, annotation, and TAINts on nodes under the node pool.
    This simplifies the operation and maintenance management of nodes.
    
- UnitedDeployment
    User can use a template to deploy the application in a different node pool. This template can support one of Kubernetes deployment, Daemonset, Statefulset.
    
### Goals
- Define the api of NodePool
- Define the api of UnitedDeployment
- Provide yurtunit-manager controller
## Proposal

- What is the plan for implementing this feature?

  Provide yurtunit-manager operator to manage NodePool and UnitedDeployment crd

### NodePool API

``` go
// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
    // The type of the nodepool
    // +optional
    Type string `json:"type,omitempty"`

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
    EdgeNodes map[string]bool `json:"edgeNodes,omitempty"`

    // Labels/annotations/taints that are currently used by nodes of the pool
    // +optional
    ManagedItems ManagedItems `json:"managedItems,omitempty"`
}

// ManagedItems contians the labels/annotations/taints that are currently
// attached to nodes of the pool
type ManagedItems struct {
    // +optional
    Labels map[string]string `json:"labels,omitempty"`

    // +optional
    Annotations map[string]string `json:"annotations,omitempty"`

    // +optional
    Taints []v1.Taint `json:"taints,omitempty"`
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

// UnitedDeploymentConditionType indicates valid conditions type of a UnitedDeployment.
type UnitedDeploymentConditionType string

const (
    // SubsetProvisioned means all the expected subsets are provisioned and unexpected subsets are deleted.
    SubsetProvisioned UnitedDeploymentConditionType = "SubsetProvisioned"
    // SubsetUpdated means all the subsets are updated.
    SubsetUpdated UnitedDeploymentConditionType = "SubsetUpdated"
    // SubsetFailure is added to a UnitedDeployment when one of its subsets has failure during its own reconciling.
    SubsetFailure UnitedDeploymentConditionType = "SubsetFailure"
)

// UnitedDeploymentSpec defines the desired state of UnitedDeployment.
type UnitedDeploymentSpec struct {
    // Selector is a label query over pods that should match the replica count.
    // It must match the pod template's labels.
    Selector *metav1.LabelSelector `json:"selector"`

    // Template describes the subset that will be created.
    // +optional
    Template SubsetTemplate `json:"template,omitempty"`

    // Topology describes the pods distribution detail between each of subsets.
    // +optional
    Topology Topology `json:"topology,omitempty"`

    // UpdateStrategy indicates the strategy the UnitedDeployment use to preform the update,
    // when template is changed.
    // +optional
    UpdateStrategy UnitedDeploymentUpdateStrategy `json:"updateStrategy,omitempty"`

    // Indicates the number of histories to be conserved.
    // If unspecified, defaults to 10.
    // +optional
    RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// SubsetTemplate defines the subset template under the UnitedDeployment.
// UnitedDeployment will provision every subset based on one workload templates in SubsetTemplate.
type SubsetTemplate struct {
    // StatefulSet template
    // +optional
    StatefulSetTemplate *StatefulSetTemplateSpec `json:"statefulSetTemplate,omitempty"`

    // Deployment template
    // +optional
    DeploymentTemplate *DeploymentTemplateSpec `json:"deploymentTemplate,omitempty"`

    // DaemonSet template
    // +optional
    DaemonSetTemplate *DaemonSetTemplateSpec `json:"daemonSetTemplate,omitempty"`
}

// StatefulSetTemplateSpec defines the subset template of StatefulSet.
type StatefulSetTemplateSpec struct {
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              appsv1.StatefulSetSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the subset template of AdvancedStatefulSet.
type DeploymentTemplateSpec struct {
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              appsv1.DeploymentSpec `json:"spec"`
}

// DaemonSetTemplateSpec defines the subset template of AdvancedStatefulSet.
type DaemonSetTemplateSpec struct {
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              appsv1.DaemonSetSpec `json:"spec"`
}

// UnitedDeploymentUpdateStrategy defines the update performance
// when template of UnitedDeployment is changed.
type UnitedDeploymentUpdateStrategy struct {
    // Includes all of the parameters a statefulset update strategy needs.
    // +optional
    StatefulSetUpdate *StatefulSetUpdate `json:"statefulSetUpdate,omitempty"`

    // Includes all of the parameters a deployment update strategy needs.
    // +optional
    DeploymentUpdate *DeploymentUpdate `json:"deploymentUpdate,omitempty"`

    // Includes all of the parameters a daemonset update strategy needs.
    // +optional
    DaemonSetUpdate *DaemonSetUpdate `json:"daemonSetUpdate,omitempty"`
}

// StatefulSetUpdate is a update strategy which allows users to control the update progress
// by providing the partition of each subset.
type StatefulSetUpdate struct {
    // Indicates strategy of statefulset for each subset.
    // +optional
    Strategies map[string]appsv1.StatefulSetUpdateStrategy `json:"strategies,omitempty"`
}

// DeploymentUpdate is a update strategy
type DeploymentUpdate struct {
    // Indicates strategy of deployment for each subset.
    // +optional
    Strategies map[string]appsv1.DeploymentStrategy `json:"strategies,omitempty"`
}

// DaemonSetUpdate is a update strategy
type DaemonSetUpdate struct {
    // Indicates strategys of daemonset for each subset.
    // +optional
    Strategies map[string]appsv1.DaemonSetUpdateStrategy `json:"strategies,omitempty"`
}

// Topology defines the spread detail of each subset under UnitedDeployment.
// A UnitedDeployment manages multiple homogeneous workloads which are called subset.
// Each of subsets under the UnitedDeployment is described in Topology.
type Topology struct {
    // Contains the details of each subset. Each element in this array represents one subset
    // which will be provisioned and managed by UnitedDeployment.
    // +optional
    Subsets []Subset `json:"subsets,omitempty"`
}

// Subset defines the detail of a subset.
type Subset struct {
    // Indicates subset name as a DNS_LABEL, which will be used to generate
    // subset workload name prefix in the format '<deployment-name>-<subset-name>-'.
    // Name should be unique between all of the subsets under one UnitedDeployment.
    // Name is NodePool Name
    Name string `json:"name"`

    // Indicates the number of the pod to be created under this subset.
    // +optional
    Replicas *int32 `json:"replicas,omitempty"`

    // Specification of the desired behavior of the pod.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
    // +optional
    PodSpec corev1.PodSpec `json:"podSpec,omitempty"`
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

    // Records the topology detail information of the replicas of each subset.
    // +optional
    SubsetReplicas map[string]int32 `json:"subsetReplicas,omitempty"`

    // SubsetUpdateStrategy
    SubsetUpdateStrategy map[string]string `json:"subsetUpdateStrategy,omitempty"`

    // The number of ready replicas.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`

    // Replicas is the most recently observed number of replicas.
    Replicas int32 `json:"replicas"`

    // TemplateType indicates the type of SubSetTemplate
    TemplateType TemplateType `json:"templateType"`
    // Total number of target subset
    // +optional
    SubsetNums int `json:"subsetNums,omitempty"`
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

- [ ] 12/13/2020: nodepool and uniteddeployment crd 
- [ ] 12/15/2020: yurtunit-manager controller 
- [ ] 12/20/2020: yurtunit-manager release 

