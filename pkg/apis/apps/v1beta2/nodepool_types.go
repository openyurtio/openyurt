/*
Copyright 2024 The OpenYurt Authors.

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

package v1beta2

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodePoolType string

// LeaderElectionStrategy represents the policy how to elect a leader Yurthub in a nodepool.
type LeaderElectionStrategy string

const (
	Edge  NodePoolType = "Edge"
	Cloud NodePoolType = "Cloud"

	ElectionStrategyMark   LeaderElectionStrategy = "mark"
	ElectionStrategyRandom LeaderElectionStrategy = "random"

	// LeaderStatus means the status of leader yurthub election.
	// If it's ready the leader elected, otherwise no leader is elected.
	LeaderStatus NodePoolConditionType = "LeaderReady"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	// The type of the NodePool
	// +optional
	Type NodePoolType `json:"type,omitempty"`

	// HostNetwork is used to specify that cni components(like flannel)
	// will not be installed on the nodes of this NodePool.
	// This means all pods on the nodes of this NodePool will use
	// HostNetwork and share network namespace with host machine.
	HostNetwork bool `json:"hostNetwork,omitempty"`

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

	// InterConnectivity represents all nodes in the NodePool can access with each other
	// through Layer 2 or Layer 3 network or not. If the field is true,
	// nodepool-level list/watch requests reuse can be applied for this nodepool.
	// otherwise, only node-level list/watch requests reuse can be applied for the nodepool.
	// This field cannot be changed after creation.
	InterConnectivity bool `json:"interConnectivity,omitempty"`

	// LeaderElectionStrategy represents the policy how to elect a leader Yurthub in a nodepool.
	// random: select one ready node as leader at random.
	// mark: select one ready node as leader from nodes that are specified by labelselector.
	// More strategies will be supported according to user's new requirements.
	LeaderElectionStrategy string `json:"leaderElectionStrategy,omitempty"`

	// LeaderNodeLabelSelector is used only when LeaderElectionStrategy is mark. leader Yurhub will be
	// elected from nodes that filtered by this label selector.
	LeaderNodeLabelSelector map[string]string `json:"leaderNodeLabelSelector,omitempty"`

	// EnableLeaderElection is used for specifying whether to enable a leader elections
	// for the nodepool. Leaders within the nodepool are elected using the election strategy and leader replicas.
	// LeaderNodeLabelSelector, LeaderElectionStrategy and LeaderReplicas are only valid when this is true.
	// If the field is not specified, the default value is false.
	EnableLeaderElection bool `json:"enableLeaderElection,omitempty"`

	// PoolScopeMetadata is used for defining requests for pool scoped metadata which will be aggregated
	// by each node or leader in nodepool (when EnableLeaderElection is set true).
	// This field can be modified. The default value is v1.services and discovery.endpointslices.
	PoolScopeMetadata []metav1.GroupVersionResource `json:"poolScopeMetadata,omitempty"`

	// LeaderReplicas is used for specifying the number of leader replicas in the nodepool.
	// If the field is not specified, the default value is 1.
	// + optional
	LeaderReplicas int32 `json:"leaderReplicas,omitempty"`
}

// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
	// Total number of ready nodes in the pool.
	// +optional
	ReadyNodeNum int32 `json:"readyNodeNum"`

	// Total number of unready nodes in the pool.
	// +optional
	UnreadyNodeNum int32 `json:"unreadyNodeNum"`

	// The list of nodes' names in the pool
	// +optional
	Nodes []string `json:"nodes,omitempty"`

	// LeaderEndpoints is used for storing the address of Leader Yurthub.
	// +optional
	LeaderEndpoints []Leader `json:"leaderEndpoints,omitempty"`

	// LeaderNum is used for storing the number of leader yurthubs in the nodepool.
	LeaderNum int32 `json:"leaderNum,omitempty"`

	// LeaderLastElectedTime is used for storing the time when the leader yurthub was elected.
	LeaderLastElectedTime metav1.Time `json:"leaderLastElectedTime,omitempty"`

	// Conditions represents the latest available observations of a NodePool's
	// current state that includes LeaderHubElection status.
	// +optional
	Conditions []NodePoolCondition `json:"conditions,omitempty"`
}

// Leader represents the hub leader in a nodepool
type Leader struct {
	// The node name of the leader yurthub
	NodeName string `json:"nodeName"`

	// The address of the leader yurthub
	Address string `json:"address"`
}

// NodePoolConditionType represents a NodePool condition value.
type NodePoolConditionType string

type NodePoolCondition struct {
	// Type of NodePool condition.
	Type NodePoolConditionType `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=nodepools,shortName=np,categories=yurt
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The type of nodepool"
// +kubebuilder:printcolumn:name="ReadyNodes",type="integer",JSONPath=".status.readyNodeNum",description="The number of ready nodes in the pool"
// +kubebuilder:printcolumn:name="NotReadyNodes",type="integer",JSONPath=".status.unreadyNodeNum"
// +kubebuilder:printcolumn:name="LeaderNodes",type="integer",JSONPath=".status.leaderNum",description="The leader node of the nodepool"
// +kubebuilder:printcolumn:name="LeaderElectionAge",type="date",JSONPath=".status.leaderElectionTime",description="The time when the leader yurthub is elected"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +genclient:nonNamespaced
// +kubebuilder:storageversion

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

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}
