---
title: NodePool Ingress Support
authors:
  - "@zzguang"
reviewers:
  - "@gnunu"
  - "@Walnux"
  - "@rambohe-ch"
  - "@wenjun93"
creation-date: 2021-06-28
last-updated: 2022-09-20
status: provisional
---

# Add Ingress Feature Support to NodePool

## Table of Contents

- [Add Ingress Feature Support to NodePool](#add-ingress-feature-support-to-nodepool)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Implementation History](#implementation-history)

## Summary

In Cloud Edge environment, many Edge devices are deployed with multi workloads so that they provide several
services to users, also some apps may need to combine with multi-services for specific usages, ingress is a
key feature to provide a unified service access interface for these usage scenarios.
Although OpenYurt can support ingress as Kubernetes from the cluster level, the distributed Edge devices are
generally managed from sub-cluster(known as NodePool in OpenYurt) level instead of the whole cluster level,
this proposal aims to fill the gap for OpenYurt NodePool.

## Motivation

When the ingress feature is supported from OpenYurt NodePool level, ingress acts as a unified interface for
services access request from outside the NodePool, it abstracts and simplifies service access logic to users,
it also reduces the complexity of NodePool services management.

### Goals

To enable the ingress feature in OpenYurt NodePool is to fill the gap that current NodePool cannot support:
- Deploy one ingress controller to the NodePool that needs to enable ingress.
- Manage the ingress controller authority from NodePool.
- Make ingress controller only cares about the related resources state of its own NodePool.
- Make ingress feature can work even in autonomy mode.
- Differentiate the ingress controllers in different NodePool by different ingress class.
- Implement it in a graceful way, it's better not to affect the current design architecture.

### Non-Goals/Future Work

Ingress controller service can be exposed through cloud provider SLB in Cloud Native environment, but in the
Edge environment, no SLB is provided, we can adopt NodePort type service at the current stage, but in future
we can evaluate whether to leverage metalLB as a feature enhancement, the reference link is shown below:

https://kubernetes.github.io/ingress-nginx/deploy/baremetal/#a-pure-software-solution-metallb

## Proposal

To implement the ingress feature support in NodePool, we need to nail down the solution design firstly.
And the design can be separated into 2 parts by our understanding:

### 1). Ingress controller deployment

It means to deploy one ingress controller to the NodePool which needs to enable the ingress feature,
we can treat it as the similar daemonset support from the NodePool level. When the ingress controller is
deployed to a NodePool, one node of the NodePool will be elected to host the deployment. Here we investigated
several possible solutions to achieve the purpose:

	1.1). Leverage YurtAppDaemon CRD to deploy ingress controller

	YurtAppDaemon seems can meet the requirement for the ingress controller deployment to specified NodePools,
	but finally we found that it's not appropriate for some kind of system components deployment like ingress
	controller, because ingress controller has a more complicated requirement than an independent deployment to
	a NodePool, it also requires to deploy the related RBAC/ConfigMap/Service/Admission Webhook, which is beyond
	the scope of YurtAppDaemon.

	1.2). Implement the deployment in NodePool operator

	Since 1.1) above is excluded, we thought to implement the NodePool ingress controller deployment in the
	NodePool operator, which aims to enhance the capability of NodePool itself, users can operate the NodePool
	CR to enable/disable the ingress feature, and it seems to be an acceptable solution, but it introduces at
	least 2 problems: one is it couples the NodePool and the components running on it, which is not a graceful
	design, the other one is users cannot operate multi NodePools ingress simultaneously, which is not very
	friendly to end users.

	1.3). Implement the deployment in a standalone operator

	To avoid the problems of 1.2) above, after several rounds of community discussions, we decided to implement
	the NodePool ingress controller deployments in a standalone operator: YurtIngress, this solution is more
	graceful and we achieved an agreement in the community meeting, we are following it for the development now.
	Thanks @Fei-Guo for his great suggestions about it.

### 2). Ingress feature implementation

Once the ingress controller has been deployed to a NodePool, this NodePool has a basic ability to provide
the ingress feature, but how to implement the ingress controller and how to adapt it to the Edge NodePool?
Firstly, to ensure ingress feature can work even in OpenYurt autonomy mode, the ingress controller should
communicate with kube-apiserver through Yurthub to cache the related resource data locally.
Then, the ingress controller should only care the resources of its own NodePool instead of whole cluster.
Here we thought about several solution alternatives and each of them has its pros and cons:
(Note that the "Edge Node" below is the elected node in a NodePool to host the ingress controller)

	2.1). Solution 1: Implement a tailored ingress controller for NodePool
					                       ------------------
					                       | kube-apiserver |
					                       ------------------
					Cloud                           ^
					--------------------------------|----------------
					Edge                            |
					        ------------------------|--------
					        |Edge Node          ----------- |
					        |                   | Yurthub | |
					        |                   -----------	|
					        |   --------------      ^       |
					        |   | ingress    |      |       |
					        |   | controller |-------       |
					        |   --------------              |
					        ---------------------------------
	Pros: Lightweight ingress controller for it is specific for NodePool.
	Cons: Big effort for ingress controller needs to monitor and manage all the related resources of
	      ingress/service/configmap/secret/endpoint/... in a NodePool by itself.

	2.2). Solution 2: Leverage and modify current opensource ingress controller to adapt NodePool
	      Take nginx ingress controller as an example:
					                          ------------------
					                          | kube-apiserver |
					                          ------------------
					Cloud                              ^
					-----------------------------------|-------------
					Edge                               |
					        ---------------------------|------
					        |Edge Node          -----------  |
					        |                   | Yurthub |  |
					        |                   -----------	 |
					        |  ---------------------   ^     |
					        |  | modified nginx    |   |     |
					        |  | ingress controller|----     |
					        |  ---------------------         |
					        ----------------------------------
	Pros: Little effort for it only needs to change some source codes to adapt NodePool.
	Cons: Intrusive to opensource ingress controller, not convenient to upgrade, it's hard to maintain in future.

	2.3). Solution 3: Leverage current opensource ingress controller and add an ingress data filter sidecar
	      By evaluating the functionality and maturity of the opensource ingress controllers, we recommend
	      to adopt nginx ingress controller:
					                                         ------------------
					                                         | kube-apiserver |
					                                         ------------------
					Cloud                                             ^
					--------------------------------------------------|------------
					Edge                                              |
					        ------------------------------------------|-------
					        |Edge Node                           ----------- |
					        |                                    | Yurthub | |
					        |                                    ----------- |
					        |  -------------------    --------------  ^      |
					        |  | nginx ingress   |    | ingress    |  |      |
					        |  | controller      |--->| datafilter |---      |
					        |  -------------------    | sidecar    |         |
					        |                         --------------         |
					        --------------------------------------------------
	Pros: None-intrusive to opensource ingress controller, convenient to upgrade, easy to maintain in future.
	Cons: Medium effort for we need to implement an ingress data filter sidecar to filter all the ingress related
	      resources of its own NodePool and communicate with nginx ingress controller for resources state update,
	      besides, for the sidecar is specific to ingress controller, it is not quite reasonable.

	2.4). Solution 4: Leverage current opensource ingress controller and add a common NodePool data filter sidecar
	      By evaluating the functionality and maturity of the opensource ingress controllers, we recommend
	      to adopt nginx ingress controller:
					                                         ------------------
					                                         | kube-apiserver |
					                                         ------------------
					Cloud                                             ^
					--------------------------------------------------|------------
					Edge                                              |
					        ------------------------------------------|-------
					        |Edge Node                           ----------- |
					        |                                    | Yurthub | |
					        |                                    ----------- |
					        |  -------------------    --------------  ^      |
					        |  | nginx ingress   |    | nodepool   |  |      |
					        |  | controller      |--->| datafilter |---      |
					        |  -------------------    | sidecar    |         |
					        |                         --------------         |
					        --------------------------------------------------
	Pros: None-intrusive to opensource ingress controller, convenient to upgrade, easy to maintain in future.
	Cons: High effort for we need to implement a common NodePool level data filter sidecar to filter all the ingress
	      resources of its own NodePool and communicate with nginx ingress controller for the resources state update,
	      besides, as a common NodePool sidecar, it needs to take all the possible resources into account in future,
	      which improves the development complexity.

	2.5). Solution 5: Similar to Solution 3 and 4, except for it leverages Yurthub data filter framework by adding
	      NodePool level ingress related resources filter in the framework, which simplifies the implementation
					                            ------------------
					                            | kube-apiserver |
					                            ------------------
					Cloud                               ^
					------------------------------------|-------------------
					Edge                                |
					        ----------------------------|----------
					        |Edge Node             -----------    |
					        |                      | Yurthub |    |
					        |                      -----------    |
					        |  -------------------      ^         |
					        |  | nginx ingress   |      |         |
					        |  | controller      |-------         |
					        |  -------------------                |
					        ---------------------------------------
	Pros: None-intrusive to opensource ingress controller, convenient to upgrade, easy to maintain in future,
	      high efficiency and less effort than Solution 3 and 4.
	Cons: Yurthub is actually a Node level instead of NodePool level sidecar, it will lead to redundant logic check
	      on the Edge Nodes which don't host the ingress controller.
	      Ingress controller is not transparent to Yurthub, Yurthub needs to check request responses to the ingress
	      controller and add specific filter operation for it.

	Conclusion:
	    By evaluating all the alternatives above, we think Yurthub naturally can take the responsibility to filter
	    the data between Cloud and Edge, so we prefer to Solution 5 currently, thanks @wenjun93 for his great
	    suggestions	about this proposal, any other suggestions will also be appreciated, if no different opinions,
	    we will follow Solution 5 to implement the feature.

### User Stories

#### Story 1
As an end user, I want to access edge services in a NodePool through http/https.
#### Story 2
As an end user, I want to develop an app which needs to utilize multi services in a NodePool.
#### Story 3
As an end user, I want to balance the traffic into a NodePool with layer-7 load-balancer mechanism and bypass
layer-4 kube-proxy.

### Implementation Details/Notes/Constraints
#### 1). Yurt-app-manager

Implement a standalone `YurtIngress` operator to support the ingress feature from NodePool level, so that
`YurtIngress` together with `NodePool/YurtAppSet/YurtAppDaemon` will compose the OpenYurt unitization capability,
and they are all managed by Yurt-app-manager. The responsibility of `YurtIngress` is to monitor all the `YurtIngress`
CRs and trigger the corresponding operations to the nginx ingress controller related components.

`YurtIngress` CRD definition:

```go
type IngressNotReadyType string

const (
	IngressPending IngressNotReadyType = "Pending"
	IngressFailure IngressNotReadyType = "Failure"
)

// IngressPool defines the details of a Pool for ingress
type IngressPool struct {
	// Indicates the pool name.
	Name string `json:"name"`

	// IngressIPs is a list of IP addresses for which nodes will also accept traffic for this service.
	IngressIPs []string `json:"ingressIPs,omitempty"`
}

// IngressNotReadyConditionInfo defines the details info of an ingress not ready Pool
type IngressNotReadyConditionInfo struct {
	// Type of ingress not ready condition.
	Type IngressNotReadyType `json:"type,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// IngressNotReadyPool defines the condition details of an ingress not ready Pool
type IngressNotReadyPool struct {
	// Indicates the base pool info.
	Pool IngressPool `json:"pool"`

	// Info of ingress not ready condition.
	Info *IngressNotReadyConditionInfo `json:"unreadyInfo,omitempty"`
}

// YurtIngressSpec defines the desired state of YurtIngress
type YurtIngressSpec struct {
	// Indicates the number of the ingress controllers to be deployed under all the specified nodepools.
	// +optional
	Replicas int32 `json:"ingressControllerReplicasPerPool,omitempty"`

	// Indicates the ingress controller image url.
	// +optional
	IngressControllerImage string `json:"ingressControllerImage,omitempty"`

	// Indicates the ingress webhook image url.
	// +optional
	IngressWebhookCertGenImage string `json:"ingressWebhookCertGenImage,omitempty"`

	// Indicates all the nodepools on which to enable ingress.
	// +optional
	Pools []IngressPool `json:"pools,omitempty"`
}

// YurtIngressCondition describes current state of a YurtIngress
type YurtIngressCondition struct {
	// Indicates the pools that ingress controller is deployed successfully.
	IngressReadyPools []string `json:"ingressReadyPools,omitempty"`

	// Indicates the pools that ingress controller is being deployed or deployed failed.
	IngressNotReadyPools []IngressNotReadyPool `json:"ingressNotReadyPools,omitempty"`
}

// YurtIngressStatus defines the observed state of YurtIngress
type YurtIngressStatus struct {
	// Indicates the number of the ingress controllers deployed under all the specified nodepools.
	// +optional
	Replicas int32 `json:"ingressControllerReplicasPerPool,omitempty"`

	// Indicates all the nodepools on which to enable ingress.
	// +optional
	Conditions YurtIngressCondition `json:"conditions,omitempty"`

	// Indicates the ingress controller image url.
	// +optional
	IngressControllerImage string `json:"ingressControllerImage"`

	// Indicates the ingress webhook image url.
	// +optional
	IngressWebhookCertGenImage string `json:"ingressWebhookCertGenImage"`

	// Total number of ready pools on which ingress is enabled.
	// +optional
	ReadyNum int32 `json:"readyNum"`

	// Total number of unready pools on which ingress is enabling or enable failed.
	// +optional
	UnreadyNum int32 `json:"unreadyNum"`
}
```
Some other details about the design:
- The `YurtIngress` CR is a cluster level instead of namespace level resource.
- Users can define different `YurtIngress` CRs to make personalized configurations for different nodepools.
  For example in the `YurtIngress` spec:
  `Replicas` aims to provide users the flexibility to set different ingress controller replicas for different pools,
  it means for a group of pools, users can deploy only 1 ingress controller replicas, but for another group of pools,
  users can deploy multiple ingress controller replicas for HA usage scenarios. If not set, the default value is 1.
  `IngressControllerImage` and `IngressWebhookCertGenImage` aim to provide users the flexibility to set nginx ingress
  controller docker images location themselves. If not set, the default value is a default image from dockerhub.
  `IngressIPs` aims to provide the capability for a specific nodepool which may need to expose the nginx ingress
  controller service through externalIPs. If not set, it will be exposed through a general NodePort service.
- In Cloud Edge usage scenarios, admission webhook should be deployed on the Cloud side for Edge network limitation,
  so when `YurtIngress Controller` tries to deploy nginx ingress controller, the upstream nginx ingress controller
  will be divided into 2 parts: one part is deployed on the Cloud side focusing on admission webhook, the other part
  is deployed to NodePool focusing on service proxy and load balancer. These 2 parts can work collaboratively without
  any conflict by setting the different startup parameters.
- A specified `ingress-nginx` namespace will be created when a NodePool ingress is enabled, then all the nginx ingress
  controller related components for different NodePools will be deployed under this namespace, it must ensure all the
  resources for all the NodePools in this namespace not have conflict with each other.
- For every ingress controller deployment in the specified NodePools, it exposes a NodePort type service for users
  to access the nginx ingress controller service.
- The ingress class is specified to be NodePool name so users can differentiate the NodePool ingress by the class.
- Nginx ingress controller will connect to kube-apiserver through Yurthub to leverage its data cache and data filter
  capability.

#### 2). Yurthub

We will leverage Yurthub data cache and data filter framework for the purposes below:
- Yurthub data cache to cache the ingress related resources data locally to ensure ingress can work in autonomy mode.
- Add ingress controller data filter implementation in Yurthub to filter the endpoints of its own NodePool:

```go
if nodePoolName, ok := currentNode.Labels[nodepoolv1alpha1.LabelCurrentNodePool]; ok {
	nodePool, err := fh.nodePoolLister.Get(nodePoolName)
	if err != nil {
		return endpoints
	}
	isNodePoolValidEps := false
	var newEpsA []v1.EndpointAddress
	for i := range endpoints.Subsets {
		for j := range endpoints.Subsets[i].Addresses {
			nodeName := endpoints.Subsets[i].Addresses[j].NodeName
			if nodeName == nil {
				continue
			}
			if inSameNodePool(*nodeName, nodePool.Status.Nodes) {
				isNodePoolValidEps = true
				newEpsA = append(newEpsA, endpoints.Subsets[i].Addresses[j])
			}
		}
		endpoints.Subsets[i].Addresses = newEpsA
		newEpsA = nil
	}
	if !isNodePoolValidEps {
		return nil
	}
}
```

## Implementation History

- [ ] 07/21/2021: Proposed idea at community meeting and collect feedbacks
- [ ] 07/21/2021: First round of feedback from community
- [ ] 09/15/2021: Second round of feedback from community
- [ ] 10/20/2021: Third round of feedback from community
- [ ] 12/08/2021: Present proposal at a community meeting
- [ ] 04/24/2022: Add YurtIngress enhancement capabilities
