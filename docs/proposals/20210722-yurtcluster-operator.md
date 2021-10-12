---
title: YurtCluster Operator
authors:
  - "@gnunu"
  - "@lindayu17"
  - "@SataQiu"
reviewers:
  - "@rambohe-ch"
  - "@guofei"
  - "@kadisi"
  - "@wenjun93"
creation-date: 2021-07-22
last-updated: 2021-08-18
status: provisional
---

# Convert Vanilla Kubernetes cluster to OpenYurt cluster through Operator

## Table of Contents
* [Convert Vanilla Kubernetes cluster to OpenYurt cluster through Operator](#convert-vanilla-kubernetes-cluster-to-openyurt-cluster-through-operator)
  * [Table of Contents](#table-of-contents)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals/Future Work](#non-goalsfuture-work)
  * [Proposal](#proposal)
    * [User Stories](#user-stories)
      * [Story 1](#story-1)
      * [Story 2](#story-2)
    * [Requirements](#requirements)
      * [Functional Requirements](#functional-requirements)
        * [FR1](#fr1)
        * [FR2](#fr2)
        * [FR3](#fr3)
        * [FR4](#fr4)
      * [Non-Functional Requirements](#non-functional-requirements)
        * [NFR1](#nfr1)
        * [NFR2](#nfr2)
      * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      * [Risks and Mitigations](#risks-and-mitigations)
  * [Alternatives](#alternatives)
  * [Upgrade Strategy](#upgrade-strategy)
  * [Additional Details](#additional-details)
    * [Test Plan](#test-plan)
  * [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

This YurtCluster Operator is to translate a vanilla Kubernetes cluster into an OpenYurt cluster, through a simple API (YurtCluster CRD).

## Motivation

To make OpenYurt translation easy for user, i.e., as automative as possible.

### Goals
* Convert nodes automatically
* Revert nodes automatically
* Translated OpenYurt cluster should be the same as through yurtctl or doing manually

### Non-Goals/Future Work

This is NOT a replacement of yurtctl command, but the convert and revert functions of yurtctl will be deprecated in the future.
And we recommend that you do the node conversion based on the declarative API of YurtCluster Operator.

## Proposal

This is an operator, with CRD and Controller.

CRD's definition:

```go
import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageMeta allows to customize the image used for components that are not
// originated from the OpenYurt release process
type ImageMeta struct {
	// Repository sets the container registry to pull images from.
	// If not set, the ImageRepository defined in YurtClusterSpec will be used instead.
	// +optional
	Repository string `json:"repository,omitempty"`
	// Tag allows to specify a tag for the image.
	// If not set, the tag related to the YurtVersion defined in YurtClusterSpec will be used instead.
	// +optional
	Tag string `json:"tag,omitempty"`
}

// ComponentConfig defines the common config for the yurt components
type ComponentConfig struct {
	// ImageMeta allows to customize the image used for the yurt component
	// +optional
	ImageMeta `json:",inline"`

	// Enabled indicates whether the yurt component has been enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ExtraArgs is an extra set of flags to pass to the OpenYurt component.
	// A key in this map is the flag name as it appears on the
	// command line except without leading dash(es).
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`
}

// YurtHubSpec defines the configuration for yurthub
type YurtHubSpec struct {
	// Cloud defines the yurthub configuration about cloud nodes
	// +optional
	Cloud YurtHubSpecTemplate `json:"cloud,omitempty"`

	// Edge defines the yurthub configuration about edge nodes
	// +optional
	Edge YurtHubSpecTemplate `json:"edge,omitempty"`
}

// YurtHubSpecTemplate defines the configuration template for yurthub
type YurtHubSpecTemplate struct {
	// ComponentConfig defines the common config for the yurt components
	// +optional
	ComponentConfig `json:",inline"`

	// PodManifestsPath defines the path to the directory on edge node containing static pod files
	// +optional
	PodManifestsPath string `json:"podManifestsPath,omitempty"`

	// KubeadmConfPath defines the path to kubelet service conf that is used by kubelet component
	// to join the cluster on the edge node
	// +optional
	KubeadmConfPath string `json:"kubeadmConfPath,omitempty"`

	// AutoRestartNodePod represents whether to automatically restart the pod after yurthub added or removed
	// +optional
	AutoRestartNodePod *bool `json:"autoRestartNodePod,omitempty"`
}

// YurtTunnelSpec defines the configuration for yurt tunnel
type YurtTunnelSpec struct {
	// ComponentConfig defines the common config for the yurt components
	// +optional
	ComponentConfig `json:",inline"`

	// ServerCount defines the replicas for the tunnel server Pod.
	// Its value should be greater than or equal to the number of API Server.
	// Operator will automatically override this value if it is less than the number of API Server.
	// +optional
	ServerCount int `json:"serverCount,omitempty"`

	// PublicIP defines the public IP for tunnel server listen on.
	// If this field is empty, the tunnel agent will use NodePort Service to connect to the tunnel server.
	// +optional
	PublicIP string `json:"publicIP,omitempty"`

	// PublicPort defines the public port for tunnel server listen on.
	// +optional
	PublicPort int `json:"publicPort,omitempty"`
}

// NodeSet defines a set of Kubernetes nodes.
// It will merge the nodes that selected by Names, NamePattern, and Selector,
// and then remove the nodes that match ExcludedNames and ExcludedNamePattern as the final set of nodes.
type NodeSet struct {
	// Names defines the node names to be selected
	// +optional
	Names []string `json:"names,omitempty"`

	// NamePattern defines the regular expression to select nodes based on node name
	// +optional
	NamePattern string `json:"namePattern,omitempty"`

	// Selector defines the label selector to select nodes
	// +optional
	Selector *corev1.NodeSelector `json:"selector,omitempty"`

	// ExcludedNames defines the node names to be excluded
	// +optional
	ExcludedNames []string `json:"excludedNames,omitempty"`

	// ExcludedNamePattern defines the regular expression to exclude nodes based on node name
	// +optional
	ExcludedNamePattern string `json:"excludedNamePattern,omitempty"`
}

// YurtClusterSpec defines the desired state of YurtCluster
type YurtClusterSpec struct {
	// ImageRepository sets the container registry to pull images from.
	// If empty, `docker.io/openyurt` will be used by default
	// +optional
	ImageRepository string `json:"imageRepository,omitempty"`

	// YurtVersion is the target version of OpenYurt
	// +optional
	YurtVersion string `json:"yurtVersion,omitempty"`

	// CloudNodes defines the node set with cloud role.
	// +optional
	CloudNodes NodeSet `json:"cloudNodes,omitempty"`

	// EdgeNodes defines the node set with edge role.
	// +optional
	EdgeNodes NodeSet `json:"edgeNodes,omitempty"`

	// YurtHub defines the configuration for yurthub
	// +optional
	YurtHub YurtHubSpec `json:"yurtHub,omitempty"`

	// YurtTunnel defines the configuration for yurt tunnel
	// +optional
	YurtTunnel YurtTunnelSpec `json:"yurtTunnel,omitempty"`
}

// Phase is a string representation of a YurtCluster Phase.
type Phase string

const (
	// PhaseInvalid is the state when the YurtCluster is invalid
	PhaseInvalid = Phase("Invalid")

	// PhaseConverting is the state when the YurtCluster is converting
	PhaseConverting = Phase("Converting")

	// PhaseDeleting is the state when the YurtCluster is deleting
	PhaseDeleting = Phase("Deleting")

	// PhaseSucceed is the state when the YurtCluster is ready
	PhaseSucceed = Phase("Succeed")
)

// YurtClusterStatusFailure defines errors states for YurtCluster objects.
type YurtClusterStatusFailure string

const (
	// InvalidConfigurationYurtClusterError indicates a state that must be fixed before progress can be made.
	InvalidConfigurationYurtClusterError YurtClusterStatusFailure = "InvalidConfiguration"
)

// NodeCondition describes the state of a node at a certain point
type NodeCondition struct {
	// The status for the condition's last transition.
	// +optional
	Status string `json:"status,omitempty"`

	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`

	// The last time this condition was updated.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The generation observed by the node agent controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// YurtClusterStatus defines the observed state of YurtCluster
type YurtClusterStatus struct {
	// Phase represents the current phase of the yurt cluster
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// FailureReason indicates that there is a problem reconciling the state, and
	// will be set to a token value suitable for programmatic interpretation.
	// +optional
	FailureReason *YurtClusterStatusFailure `json:"failureReason,omitempty"`

	// FailureMessage indicates that there is a fatal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// NodeConditions holds the info about node conditions
	// +optional
	NodeConditions map[string]NodeCondition `json:"nodeConditions,omitempty"`

	// The generation observed by the operator controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// YurtCluster is the Schema for the yurtclusters API
type YurtCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YurtClusterSpec   `json:"spec,omitempty"`
	Status YurtClusterStatus `json:"status,omitempty"`
}
```

The CRD would be enforced to have a cluster singleton CR semantics, through patched name validation for CRD definition. (for kubebuilder, under config/crd/patches)

The controller would listen incomming CR, and analyze the requirements to figure out user's intention, that is, what nodes to convert, and what nodes to revert.

The controller would update status to record converted, reverted, and failed nodes.

The plan:
This feature would be merged into release 0.7, hopefully.

### User Stories

#### Story 1
To convert all nodes, including the future added nodes, automatically.

#### Story 2
To revert some nodes back to normal K8S node.

### Requirements

User should have a Kubernetes cluster setup.

#### Functional Requirements

The Operator would do these things:
1) Analyze requests
2) Do nodes conversion
3) Do nodes reversion
4) Act on CR deletion

##### FR1
Analyze requests:
Through the YurtCluster CR, the controller would figure out what nodes to convert, what nodes to revert, and if enabling addons like YurtHub.
Th nodes to be converted or reverted can be specified using Regular expressions, the controller would process the nodes set, including those coming in the future.

##### FR2
Convert:
From the analysis, the controller would know which nodes should be converted and do conversion automatically.

##### FR3
Revert:
From the analysis, the controller would know which nodes should be reverted and do reversion automatically.

##### FR4
On CR deletion, the controller should undo anything done to the original K8S cluster, that is, recover back to the origin point.

#### Non-Functional Requirements

##### NFR1
This operator should not cause any performance impact to the cluster.

##### NFR2
This operator should keep cluster serects safe.

### Implementation Details/Notes/Constraints

The procedure of coverting/reverting a node will refer to yurtctl's implementation.

Whether or not use helm is to be discovered.

### Risks and Mitigations

If number of nodes is large, the performance may be a concern.
Solution: reduce non-necessary actions as low as possible, and keep necessary cache to reduce api-server/etcd access.

## Alternatives

yurtctl: the classic command to do the cluster conversion/reversion.
manual: user should convert/revert nodes manually.

## Upgrade Strategy

The Operator supports OpenYurt components upgrade, such as YuntHub.

For Operator itself, upgrade would have no side effects, idempotent semantics would be kept.

## Additional Details

### Test Plan

test minikube
test kind
test kubeadm
test Operator upgrade

## Implementation History

[x] 03/27/2021: Proposed idea in an issue (https://github.com/openyurtio/openyurt/issues/328).
[x] 08/18/2021: Refine and update the YurtCluster Spec definition.
