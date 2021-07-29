---
title: YurtCluster Operator
authors:
  - "@gnunu"
  - "@lindayu17"
reviewers:
  - "@rambohe-ch"
  - "@guofei"
  - "@kadisi"
  - "@wenjun93"
creation-date: 2021-07-22
last-updated: 2021-07-22
status: provisional
---

# Convert Vanilla Kubernetes cluster to OpenYurt cluster through Operator

## Table of Contents
* [Convert Vanilla Kubernetes cluster to OpenYurt cluster through Opertaor](#convert-vanilla-kubernetes-cluster-to-openyurt-cluster-through-operator)
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

This is NOT a replacement of yurtctl command.

## Proposal

This is an operator, with CRD and Controller.

CRD's definition:

```go
type ComponentConfig struct {
        Enabled bool   `json:"enebled,omitempty"`
        Version string `json:"version,omitempty"`
}

type NodeSpec struct {
        Names           []string `json:"names,omitempty"`
        Pattern         string   `json:"pattern,omitempty"`
        ExcludedNames   []string `json:"excludedNames,omitempty"`
        ExcludedPattern string   `json:"excludedPattern,omitempty"`
}

type FailedNode struct {
        Name string `json:"name,omitempty"`
        Info string `json:"info,omitempty"`
}

// YurtClusterSpec defines the desired state of YurtCluster
type YurtClusterSpec struct {
        CloudNodes            NodeSpec        `json:"cloudNodes,omitempty"`
        EdgeNodes             NodeSpec        `json:"edgeNodes,omitempty"`
        YurtHub               ComponentConfig `json:"yurtHub,omitempty"`
        YurtControllerManager ComponentConfig `json:"yurtControllerManager,omitempty"`
        TunnelService         ComponentConfig `json:"yurtTunnel,omitempty"`
        AppManager            ComponentConfig `json:"appManger,omitempty"`
}

// YurtClusterStatus defines the observed state of YurtCluster
type YurtClusterStatus struct {
        ConvertedCloudNodes   []string        `json:"convertedCloudNodes,omitempty"`
        ConvertedEdgeNodes    []string        `json:"convertedEdgeNodes,omitempty"`
        YurtHub               ComponentConfig `json:"yurtHub,omitempty"`
        YurtControllerManager ComponentConfig `json:"yurtControllerManager,omitempty"`
        YurtTunnel            ComponentConfig `json:"yurtTunnel,omitempty"`
        AppManager            ComponentConfig `json:"appManger,omitempty"`
        ExcludedNodes         []string        `json:"excludedNodes,omitempty"`
        FailedNodes           []FailedNode    `json:"failedNodes,omitempty"`
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
Through the YurtCluster CR, the controller would figure out what nodes to convert, what nodes to revert, and if enabling addons like YurtTunel and AppManger.
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

Wheteher or not use helm is to be discovered.

### Risks and Mitigations

If number of nodes is large, the performance may be a concern.
Solution: reduce non-necessary actions as low as possible, and keep necessary cache to reduce api-server/etcd access.

## Alternatives

yurtctl: the classic command to do the cluster conversion/reversion.
manual: user should convert/revert nodes manually.

## Upgrade Strategy

The Operator supports OpenYurt components upgrade, such as YuntTunnel and AppManager.

For Operator itself, upgrade would have no side effects, idempotent semantics would be kept.

## Additional Details

### Test Plan

test minikube
test kind
test kubeadm
test Operator upgrade

## Implementation History

[x] 03/27/2021: Proposed idea in an issue (https://github.com/openyurtio/openyurt/issues/328).
