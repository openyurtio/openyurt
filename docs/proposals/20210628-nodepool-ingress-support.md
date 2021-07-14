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
last-updated: 2021-06-28
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
1). Ingress controller deployment
	It means to deploy one ingress controller to the NodePool which needs to enable the ingress feature,
	we can treat it as similar daemonset support from the NodePool level. When the ingress controller is
	deployed to a NodePool, one node of the NodePool will be elected to host the deployment. The related
	development is working in process by @kadisi and it is supposed to be open sourced soon.

2). Ingress feature implementation
	Once the ingress controller has been deployed to a NodePool, this NodePool has a basic ability to provide
	the ingress feature, but how to implement the ingress controller and how to adapt it to NodePool?
	Firstly, to ensure the ingress feature can work even in OpenYurt autonomy mode, the ingress controller
	should communicate with kube-apiserver through Yurthub to cache the related resource data.
	Then, the ingress controller should only monitor the resources of its own NodePool instead of whole cluster.
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
	Pros: Lightweight ingress controller for it is specific for NodePool
	Cons: Big effort for ingress controller needs to monitor and manage all the related resources of
	      ingress/service/configmap/secret/endpoint in a NodePool by itself

	2.2). Solution 2: Leverage and modify current opensource ingress controller to adapt NodePool
	      Take Nginx ingress controller as an example:
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
	Pros: Little effort for it only needs to change some source codes to adapt NodePool
	Cons: Intrusive to opensource ingress controller, not convenient to upgrade, it's hard to maintain in future

	2.3). Solution 3: Leverage current opensource ingress controller and add a sidecar to adapt NodePool
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
					        |  -------------------    -------------   ^      |
					        |  | nginx ingress   |    | ingress   |   |      |
					        |  | controller      |--->| controller|----      |
					        |  -------------------    | sidecar   |          |
					        |                         -------------          |
					        --------------------------------------------------
	Pros: None-intrusive to opensource ingress controller, convenient to upgrade, easy to maintain in future
	Cons: Medium effort for we need to implement an ingress controller sidecar to filter all the ingress related
	      resources of its own NodePool and communicate with nginx ingress controller for resources state update,
	      besides, for the sidecar will intercept all the traffic to nginx ingress controller, it improves the
	      development complexity and may lead to some efficiency loss

	2.4). Solution 4: Similar to Solution 3, except for it depends on Yurthub to filter the ingress related
	      resources of its own NodePool, which simplifies the implementation of sidecar
					                            ------------------
					                            | kube-apiserver |
					                            ------------------
					Cloud                               ^
					------------------------------------|--------------------------
					Edge                                |
					        ----------------------------|---------------------
					        |Edge Node             -----------               |
					        |                      | Yurthub |               |
					        |                      -----------               |
					        |  -------------------   ^    ^   -------------  |
					        |  | nginx ingress   |   |    |   | ingress   |  |
					        |  | controller      |----    ----| controller|  |
					        |  -------------------            | sidecar   |  |
					        |                                 -------------  |
					        --------------------------------------------------
	Pros: None-intrusive to opensource ingress controller, convenient to upgrade, easy to maintain in future,
	      high efficiency and less effort than Solution 3
	Cons: Ingress controller is not transparent to Yurthub, Yurthub needs to check requests from ingress controller
	      and add specific filter operation for it

	Conclusion:
		By evaluating all the alternatives above, we think Yurthub naturally can take the responsibility to filter the
		data between Cloud and Edge, so we prefer to Solution 4 currently, thanks @wenjun93 for his great suggestions
		about this proposal, any other suggestions will also be appreciated, if no different opinions, we will follow
		Solution 4 to implement the feature.

### User Stories

#### Story 1
As an end user, I want to access edge services in a NodePool through http/https.
#### Story 2
As an end user, I want to develop an app which needs to utilize multi services in a NodePool.
#### Story 3
As an end user, I want to balance the traffic into a NodePool with layer-7 load-balancer mechanism and bypass
layer-4 kube-proxy.

### Implementation Details/Notes/Constraints
1). Yurt-app-manager:
	- When a NodePool is created, it can be labeled whether to enable the ingress feature, for example:
	  ```yaml
            nodepool.openyurt.io/ingress-enable: "true"
	  ```
	  Whether to enable ingress by default when NodePool created is TBD.
	- A new CRD will be defined to provide the daemonset support from NodePool level, so that one ingress controller
	  can be deployed to the NodePool which needs to enable ingress feature.
	- For every ingress controller deployment in different NodePools, it creates a NodePort type service to expose the
	  ingress controller service, and binds a corresponding configmap to differentiate the NodePools. All the ingress
	  controller deployments for different NodePools can use one same service account.
	- Add 2 NodePool annotations for users to configure the ingress feature when NodePool is created:
	  ```yaml
            nodepool.openyurt.io/ingress-ips: "xxx"      # the ingress controller service IP scope
            nodepool.openyurt.io/ingress-config: "xxx"   # the ingress controller nginx config
	  ```

2). Nginx ingress controller:
	- Keep the upstream version none-intrusive, so that it will be very convenient to upgrade in future.
	- It specifies the ingress class to NodePool name so the ingress CR can select the corresponding ingress controller.
	- It should be granted the authorities to create/update/patch the related resources in kube-system namespace.
	- It connects with Yurthub to monitor the resources(ingress/service/configmap/secret/endpoint) of its own NodePool.

3). Ingress controller sidecar:
	- When sidecar is launched, it gets the NodePool id it belongs to, and connects to Yurthub to list/watch the NodePool.
	- When it detects the NodePool annotation below is changed, it will update externalIPs of the corresponding service:
	  ```yaml
            nodepool.openyurt.io/ingress-ips: "xxx"
	  ```
	- When it detects the NodePool annotation below is changed, it will update the corresponding configmap of the NodePool,
	  which will trigger the ingress controller to reload nginx config for the resource state sync.
	  ```yaml
            nodepool.openyurt.io/ingress-config: "xxx"
	  ```

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR
