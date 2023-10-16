---
title: Proposal for edge appraisal
authors:
  - "@my0sotis"
reviewers:
  - ""
creation-date: 2023-10-14
last-updated: 2023-10-14
status: provisional
---

# Edge Computing platform Evaluation System

## Table of Contents

- [Edge Computing platform Evaluation System](#edge-computing-platform-evaluation-system)
	- [Table of Contents](#table-of-contents)
	- [Glossary](#glossary)
		- [EdgeAppraisal](#edgeappraisal)
	- [Summary](#summary)
	- [Motivation](#motivation)
		- [Goals](#goals)
		- [Non-Goals/Future Work](#non-goalsfuture-work)
	- [Proposal](#proposal)
		- [Covered Scenarios](#covered-scenarios)
			- [Cloud-Edge Communication](#cloud-edge-communication)
			- [Edge Resource](#edge-resource)
			- [Cloud-Edge Operation](#cloud-edge-operation)
			- [Edge Autonomy](#edge-autonomy)
		- [EdgeAppraisal API](#edgeappraisal-api)
		- [Implementation Details](#implementation-details)
			- [EdgeAppraisal Validating Webhook](#edgeappraisal-validating-webhook)
			- [EdgeApprisal Controller](#edgeapprisal-controller)
		- [User Stories](#user-stories)
			- [Story 1 (Traffic)](#story-1-traffic)
			- [Story 2 (Autonomy)](#story-2-autonomy)
			- [Story 3 (Operation)](#story-3-operation)
			- [Story 4 (Resource)](#story-4-resource)
		- [Comparison with existing open source projects](#comparison-with-existing-open-source-projects)
			- [Kubemark](#kubemark)
			- [ChaosMesh](#chaosmesh)
	- [Implementation History](#implementation-history)

## Glossary

### EdgeAppraisal

EdgeAppraisal is a new CRD to customize the configuration of the appraisal for one aspect of the Edge Computing Platform feature. It provides a simple way to configure the test parameters.

## Summary

Leveraging the robust container orchestration and scheduling capabilities inherent to native Kubernetes, OpenYurt has developed a comprehensive suite of edge cloud-native solutions without compromising the integrity of the native Kubernetes environment. These solutions encompass crucial features like edge autonomy and edge operation and maintenance. However, during the course of technological evolution, code modifications can inadvertently impact the functionality of these components. This proposal seeks to introduce an automated testing platform capable of assessing numerous features essential for cloud-edge scenarios.

## Motivation

Given the unique nature of the cloud-edge scenario, multiple components were introduced to extend native Kubernetes to the edge. However, as technology evolves, code modifications can inadvertently impact the functionality of these components. Presently, our approach to circumventing impaired component functionality involves manual deployment and observation, a process that is both time-consuming and labor-intensive.Therefore, we design a Edge Computing Platform Evaluation System by introducing EdgeAppraisal CRD, relevant controller and webhooks. Different test processes will be automatically performed based on the configured EdgeAppraisal.

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

### Covered Scenarios

#### Cloud-Edge Communication

In the cloud-edge scenario, the edge nodes monitor the changes of endpoint epslices of all nodes, and when Pods are massively deleted and re-created, this can lead to a substantial surge in communication traffic between the cloud and the edge. Such an increase significantly escalates the cost of public network traffic for customers and imposes constraints on the overall cluster size. Hence, we prioritize assessing the API Server's traffic volume in scenarios involving the extensive creation and deletion of Pods and Servers, with the objective of keeping the traffic at a relatively low level.

#### Edge Resource

When considering resource consumption at the edge, the primary focus is on the lightweight nature of cloud-edge collaboration components. In the context of cloud-edge collaboration scenarios, edge nodes often operate with limited resources. These nodes have modest resource allocations available for the cloud-edge collaboration platform components. The objective is to minimize hardware resource demands for the cloud-edge collaboration components and, in turn, reduce costs.

To achieve this, we propose deploying an equal number of pods to the edge nodes using a DaemonSet. In this configuration, the distinguishing factor among edge nodes is the specific Cloud Edge Collaboration system components they host. The approach involves monitoring the resource consumption of edge nodes and assessing the overall resource utilization when these nodes are tasked with managing a designated number of pods.

#### Cloud-Edge Operation

In a native Kubernetes cluster, the cloud control component needs to directly access the Kubelet on the edge node to execute operation and maintenance commands, or the cloud operation and maintenance monitoring component metrics-server needs to pull monitoring metrics data from the cloud for the edge. In the case of an edge-hosted cluster, when your edge nodes are deployed on the intranet, the cloud cannot access the edge nodes directly.

To maintain operational consistency with native Kubernetes, it is imperative to ensure that certain operation and maintenance commands, like 'kubectl logs' and 'kubectl exec', remain effective in the context of cloud-edge collaboration scenarios. Additionally, users may need to issue multiple operation and maintenance commands to a single cluster or node concurrently, making the management of cluster operation and maintenance concurrency a critical aspect of cloud-side operations and maintenance.

#### Edge Autonomy

In a native Kubernetes, typically if a node is disconnected from the api-server, it is not possible to recover a running Pod when the node fails. furthermore, when a node's heartbeat goes unreported for more than 5m, the Pods on the edge node are evicted by the native controller of Kube-Controller-Manager component. This presents a notable challenge for the cloud-edge collaboration architecture due to the inherent unreliability of the cloud-edge network. Ensuring that edge nodes can consistently offer reasonably reliable services in the face of network instability is essential. This aspect of the metrics primarily assesses the nodes' ability to maintain service provision after network disconnections.

### EdgeAppraisal API

In order to create a test for a certain scenario, you need to create an EdgeAppraisal resource and configure it. The EdgeAppraisal resource is defined as follows:

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

### Implementation Details

#### EdgeAppraisal Validating Webhook

The EdgeAppraisal Validating Webhook is employed to prevent alterations to ongoing tests. As a test process isn't an atomic operation, making changes during testing can lead to unforeseen errors.

#### EdgeApprisal Controller

1. Execute distinct test processes based on the Spec.Type.
2. Upon the completion of the test process, whether it ends in success or failure, gather the test results and update them within the EdgeAppraisal Status.

### User Stories

#### Story 1 (Traffic)

As an end user, I aim to evaluate the communication traffic volume within the cloud-edge network.

```yaml
apiVersion: apps.yurt.io/v1alpha1
kind: EdgeAppraisal
metadata:
  name: traffic-test
spec:
  type: traffic
  config:
    pod_num: 500
    svc_num: 30 
  timeout: 300
```

#### Story 2 (Autonomy)

As an end user, I aim to assess the autonomy capabilities of the edge node.

```yaml
apiVersion: apps.yurt.io/v1alpha1
kind: EdgeAppraisal
metadata:
  name: autonomy-test
spec:
  type: autonomy
  config:
    disconnect_time: 10
  timeout: 300
```

#### Story 3 (Operation)

As an end user, I seek to evaluate the performance of cloud-edge operation and maintenance capabilities through testing.

```yaml
apiVersion: apps.yurt.io/v1alpha1
kind: EdgeAppraisal
metadata:
  name: operation-test
spec:
  type: operation
  config:
    concurrency: 10
  timeout: 300
```

#### Story 4 (Resource)

As an end user, I intend to assess the resource consumption at the edge through testing.

```yaml
apiVersion: apps.yurt.io/v1alpha1
kind: EdgeAppraisal
metadata:
  name: resource-test
spec:
  type: resource
  config:
    pod_per_node: 10
  timeout: 300
```

### Comparison with existing open source projects

#### Kubemark

Kubemark is a tool specifically tailored for evaluating the scalability and performance of Kubernetes. It operates as a Kubernetes cluster within a Kubernetes cluster, offering an easy-to-deploy and user-friendly solution capable of scaling to thousands of nodes.

Kubemark primarily simulates the kubelet and kube proxy components on real nodes but doesn't provide simulation capabilities for other components deployed by OpenYurt on edge nodes. Furthermore, due to its construction, it falls short in assessing the functionality and performance of edge node components, which are also a very important part in the cloud edge scenario.

#### ChaosMesh

Chaos Mesh is an open-source cloud-native Chaos Engineering platform. It provides a wide range of fault simulation options and boasts a substantial capacity to orchestrate fault scenarios. With Chaos Mesh, you can effectively replicate various real-world abnormalities encountered during development, testing, and production, uncovering potential system issues.

However, the scenarios we need to test don't solely encompass abnormal situations within the system but also include monitoring and assessment of indicators during normal operations. While ChaosMesh excels at simulating system anomalies, it may necessitate additional steps to evaluate the performance of normal operating indicators.

## Implementation History

- [ ] EdgeAppraisal API CRD
- [ ] EdgeAppraisal Controller
- [ ] Edge Resource
- [ ] Cloud-Edge Communication
- [ ] Cloud-Edge Operation
- [ ] Edge Autonomy

