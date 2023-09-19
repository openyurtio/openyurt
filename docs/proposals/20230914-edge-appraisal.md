---
title: Proposal for edge appraisal
authors:
  - "@my0sotis"
reviewers:
  - ""
creation-date: 2023-09-14
last-updated: 2023-09-14
status: provisional
---

# Edge Computing platform Evaluation System

## Table of Contents

- [Title](#Edge Computing platform Evaluation System)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
    - [Requirements (Optional)](#requirements-optional)
      - [Functional Requirements](#functional-requirements)
        - [FR1](#fr1)
        - [FR2](#fr2)
      - [Non-Functional Requirements](#non-functional-requirements)
        - [NFR1](#nfr1)
        - [NFR2](#nfr2)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
    - [Test Plan [optional]](#test-plan-optional)
  - [Implementation History](#implementation-history)

## Glossary

### EdgeAppraisal

EdgeAppraisal is a new CRD to customize the configuration of the appraisal for one aspect of the Edge Computing Platform feature. It provides a simple way to configure the test parameters.

## Summary

This proposal aims to present an automated test platform capable of testing for a number of features required for cloud-side-end scenarios.

## Motivation

Due to the specificity of the cloud-side-end scenario, several components were proposed to extend the native Kubernetes to edge. However, in the process of technology iteration, code changes may result in impaired functionality of components. Currently, our method of avoiding impaired component functionality is to manually deploy and observe, which is both time consuming and labor intensive. Therefore, we design a Edge Computing Platform Evaluation System by introducing EdgeAppraisal CRD, relevant controller and webhooks. Different test processes will be automatically performed based on the configured EdgeAppraisal.

### Goals

Support automated testing of the Cloud Edge Collaboration Platform, we want to cover the following features:

- Cloud-Edge Communication
- Edge resource
- Cloud-Edge Operation
- Edge Autonomy

### Non-Goals/Future Work

Non-goals are limited to the scope of this proposal. These scenarios may evolve in the future:

- E2E test
- IoT support
- network capabilities
- multi-region workloads management

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

- What is the plan for implementing this feature?
- What data model changes, additions, or removals are required?
- Provide a scenario, or example.
- Use diagrams to communicate concepts, flows of execution, and states.

### Cloud-Edge Communication

In the cloud-edge scenario, the edge nodes listen to the changes of endpoint epslices of all nodes, and when Pods are massively deleted and re-created, the communication traffic between the cloud and the edge will increase dramatically, which is a significant increase in the cost of public network traffic for the customers, and this also limits the overall cluster size. Therefore, we consider the traffic size of API Server in the case of large-scale creation/deletion of Pods and Servers, and expect the traffic size to be maintained at a relatively low level.

### Edge Resource

The edge resource consumption consideration is mainly the lightweight of the cloud edge collaboration components. In the cloud edge collaboration scenario, the edge nodes are often resource-constrained devices, and the resources that can be allocated to the cloud edge collaboration platform components are relatively small, but also to avoid the demand for hardware resources of the cloud edge collaboration components and reduce costs. Therefore, consider deploying the same number of pods to the edge nodes through DaemonSet, at this time the difference between the edge nodes lies in the different Cloud Edge Collaboration system components, observing the resource consumption of the edge nodes, and evaluating the overall resource consumption when the edge nodes are controlling the specified number of pods.

### Cloud-Edge Operation

In a native Kubernetes cluster, the cloud control component needs to directly access the Kubelet on the edge node to execute operation and maintenance commands, or the cloud operation and maintenance monitoring component metrics-server needs to pull monitoring metrics data from the cloud for the edge. In the case of an edge-hosted cluster, when your edge nodes are deployed on the intranet, the cloud cannot access the edge nodes directly. In order to ensure the unity of operation and maintenance capabilities with the native K8s, you need to ensure that some operation and maintenance commands of the native K8s, such as the kubectl logs/exec commands, can also take effect in the cloud-edge collaboration scenario. At the same time, users may send multiple operation and maintenance commands to a single cluster or a single node at the same time, the concurrency of cluster operation and maintenance is also a part of the cloud-side operation and maintenance needs to pay attention to.

### Edge Autonomy

In a native Kubernetes, typically if a node is disconnected from the api-server, it is not possible to recover a running Pod when the node fails. furthermore, when a node's heartbeat goes unreported for more than 5m, the Pods on the edge node are evicted by the native controller of Kube-Controller-Manager component. This poses a significant challenge to the cloud-edge collaboration architecture since the cloud-edge network can be unreliable. It is necessary to ensure that the edge nodes can continue to provide relatively reliable services despite network fluctuations, and this part of the metrics mainly evaluates how well the nodes continue to provide services after network disconnection.

### EdgeAppraisal API

```go
type AppraisalType string

const (
	AppraisalTraffic   AppraisalType = "traffic"
	AppraisalAutonomy  AppraisalType = "autonomy"
	AppraisalOperation AppraisalType = "operation"
	AppraisalResource  AppraisalType = "resource"
)

// EdgeAppraisalSpec defines the desired state of EdgeAppraisal
type EdgeAppraisalSpec struct {
	Type    AppraisalType     `json:"type"`
	Config  map[string]string `json:"config"`
	Timeout int               `json:"timeout"`
}

type EdgeAppraisalConditionType string

const (
	// EdgeAppraisalInit means the EdgeAppraisal is initialized
	EdgeAppraisalInit EdgeAppraisalConditionType = "Init"
	// EdgeAppraisalFailed means the EdgeAppraisal has failed its execution.
	EdgeAppraisalFailed EdgeAppraisalConditionType = "Failed"
	// EdgeAppraisalCompleted means the EdgeAppraisal has completed
	EdgeAppraisalCompleted EdgeAppraisalConditionType = "Completed"
	// EdgeAppraisalProgressing means the EdgeAppraisal is still progressing
	EdgeAppraisalProgressing EdgeAppraisalConditionType = "Progressing"
)

// EdgeAppraisalCondition describes current state of a EdgeAppraisal.
type EdgeAppraisalCondition struct {
	// Type of EdgeAppraisal condition
	Type EdgeAppraisalConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`

	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message"`
}

// EdgeAppraisalStatus represents the current state of a single EdgeAppraisal.
type EdgeAppraisalStatus struct {
	// The latest available observations of an object's current state.
	Conditions []EdgeAppraisalCondition `json:"conditions"`

	// Conclusions means single Appraisal returns values
	Conclusions map[string]string `json:"conclusions"`

	// Represents time when the EdgeAppraisal controller started processing an EdgeAppraisal. When an
	// EdgeAppraisal is created in the suspended state, this field is not set until the
	// first time it is resumed. This field is reset every time a EdgeAppraisal is resumed
	// from suspension. It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime"`

	// Represents time when the EdgeAppraisal was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// The completion time is only set when the job finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// EdgeAppraisal is the Schema for the edgeappraisals API
type EdgeAppraisal struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeAppraisalSpec   `json:"spec,omitempty"`
	Status EdgeAppraisalStatus `json:"status,omitempty"`
}
```

### User Stories

- Detail the things that people will be able to do if this proposal is implemented.
- Include as much detail as possible so that people can understand the "how" of the system.
- The goal here is to make this feel real for users without getting bogged down.

#### Story 1

As an end user, I want to 

#### Story 2

### Implementation Details

- What are some important details that didn't come across above.
- What are the caveats to the implementation?
- Go in to as much detail as necessary here.
- Talk about core concepts and how they releate.

### Comparison with existing open source projects

#### Kubemark


#### ChaosMesh


## Implementation History

- [ ] EdgeAppraisal API CRD
- [ ] EdgeAppraisal Controller
- [ ] Edge Resource
- [ ] Cloud-Edge Communication
- [ ] Cloud-Edge Operation
- [ ] Edge Autonomy

