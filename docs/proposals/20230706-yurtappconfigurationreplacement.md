---
title: Proposal about YurtAppConfigurationReplacement
authors:
  - "@vie-serendipity"
reviewers:
  - ""
creation-date:
last-updated:
status:
---

# Proposal about YurtAppConfigurationReplacement
- [Proposal about YurtAppConfigurationReplacement](#proposal-about-yurtappconfigurationreplacement)
	- [Glossary](#glossary)
		- [YurtAppConfigurationReplacement](#yurtappconfigurationreplacement)
	- [Summary](#summary)
	- [Motivation](#motivation)
		- [Goals](#goals)
		- [Non-Goals/Future Work](#non-goalsfuture-work)
	- [Proposal](#proposal)
		- [Inspiration](#inspiration)
		- [YurtAppConfigurationReplacement API](#yurtappconfigurationreplacement-api)
		- [Architecture](#architecture)
		- [Implementation Details](#implementation-details)
			- [Deployment Mutating Webhook](#deployment-mutating-webhook)
				- [Prerequisites for webhook (Resolving circular dependency)](#prerequisites-for-webhook-resolving-circular-dependency)
				- [Workflow of mutating webhook](#workflow-of-mutating-webhook)
			- [YurtAppConfigurationReplacement Validating Webhook](#yurtappconfigurationreplacement-validating-webhook)
			- [YurtAppConfigurationReplacement Controller](#yurtappconfigurationreplacement-controller)
				- [Task 1](#task-1)
				- [Task 2](#task-2)
		- [User Stories](#user-stories)
			- [Story 1](#story-1)
			- [Story 2](#story-2)
	- [Implementation History](#implementation-history)

## Glossary
### YurtAppConfigurationReplacement
YurtAppConfigurationReplacement is a new CRD used to personalize the configuration of the workloads managed by YurtAppSet/YurtAppDaemon. It provides an simple and straightforward way to configure every field of the workload under each nodepool. 
## Summary
Due to the objective existence of heterogeneous environments such as resource configurations and network topologies in each geographic region, the configuration is always different in each region. The workloads(Deployment/StatefulSet) of nodepools in different regions can be rendered through simple configuration by using YurtAppConfigurationReplacement which also supports multiple resources. 
## Motivation
YurtAppDaemon is proposed for homogeneous workloads. Yurtappset is not user-friendly and scalable, although it can be used for workload configuration by patch field. Therefore, we expect to render different configurations for each workload easily, including replicas, images, configmap, secret, pvc, etc. In addition, it is essential to support rendering of existing resources, like YurtAppSet and YurtAppDaemon, and future resources. 
### Goals
- Define the API of YurtAppConfigurationReplacement
- Provide YurtAppConfigurationReplacement controller
- Provide Deployment mutating webhook
- Provide YurtAppConfigurationReplacement validating webhook
### Non-Goals/Future Work
- StatefulSet mutating webhook
- Optimize YurtAppSet(about patch)
## Proposal
### Inspiration
Reference to the design of ClusterRole and ClusterRoleBinding. 

1. Considering the simplicity of personalized rendering configuration, an incremental-like approach is used to implement injection, i.e., only the parts that need to be modified need to be declared. They are essentially either some existing resources, such as ConfigMap, Secret, etc., or some custom fields such as Replicas, Env, etc. Therefore, it is reasonable to abstract these configurable fields into an Replacement. The design of Replacement refers to the design of VolumeSource in kubernetes. 
2. In order to inject Replacement into the workloads, we should create a new CRD, which works as below. 
<img src = "../img/yurtappconfigurationreplacement/Inspiration.png" width="600">

### YurtAppConfigurationReplacement API
1. YurtAppConfigurationReplacement needs to be bound to YurtAppSet/YurtAppDaemon.
Considering that there are multiple Deployment/StatefulSet per nodepool, as shown below, it must be bound to YurtAppSet/YurtAppDaemon for injection. We use subject field to bind it to YurtAppSet/YurtAppDaemon. 


1. YurtAppConfigurationReplacement is only responsible for injection of an Replacement, which means for each node pool that we want to personalize, we need to create a new YurtAppConfigurationReplacement resource. 

```go
// ImageReplacement specifies the corresponding container and the claimed image.
type ImageReplacement struct {
	ContainerName string `json:"containerName"`
	// ImageClaim represents the image name which is used by container above.
	ImageClaim string `json:"imageClaim"`
}

// EnvReplacement specifies the corresponding container and Env
type EnvReplacement struct {
	ContainerName string `json:"containerName"`
	// EnvClaim represents the detailed enviroment variables that container contains.
	EnvClaim map[string]string `json:"envClaim"`
}

type PersistentVolumeClaimReplacement struct {
	ContainerName string `json:"containerName"`
	// PVCSource represents volume name.
	PVCSource     string `json:"pvcSource"`
	// PVCTarget represents the PVC corresponding to the volume above.
	PVCTarget     string `json:"pvcTarget"`
}

type ConfigMapReplacement struct {
	// ContainerName represents name of the container.
	ContainerName string `json:"containerName"`
	// ConfigMapSource represents volume name.
	ConfigMapSource string `json:"configMapClaim"`
	// ConfigMapTarget represents the ConfigMap corresponding to the volume above.
	ConfigMapTarget string `json:"configMapTarget"`
}

type SecretReplacement struct {
	// ContainerName represents name of the container.
	ContainerName string `json:"containerName"`
	// SecretSource represents volume name.
	SecretSource string `json:"secretClaim"`
	// SecretTarget represents the Secret corresponding to the volume above.
	SecretTarget string `json:"secretTarget"`
}

// Replacement represents configuration to be injected.
// Only one of its members may be specified.
type Replacement struct {
	Image                 *ImageReplacement                 `json:"image"`
	ConfigMap             *ConfigMapReplacement             `json:"configMap"`
	Secret                *SecretReplacement                `json:"secret"`
	Env                   *EnvReplacement                   `json:"env"`
	PersistentVolumeClaim *PersistentVolumeClaimReplacement `json:"persistentVolumeClaim"`
	Replicas              *int                              `json:"replicas"`
	UpgradeStrategy       *string                           `json:"upgradeStrategy"`
}

type Subject struct {
	metav1.TypeMeta `json:",inline"`
	// Name is the name of YurtAppSet or YurtAppDaemon
	Name string `json:"name"`
	// Pools represent names of nodepool that replacements will be injected into.
	Pools []string `json:"pools"`
}

type YurtAppConfigurationReplacement struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Describe the object to which replacements belongs
	Subject Subject `json:"subject"`
	// Describe detailed configuration to be injected of the subject above.
	Replacements []Replacement `json:"replacements"`
}
```
### Architecture
The whole architecture is shown below. 
<img src = "../img/yurtappconfigurationreplacement/Architecture.png" width="800">

### Implementation Details
#### Deployment Mutating Webhook
##### Prerequisites for webhook (Resolving circular dependency)
Since YurtManager is deployed as a Deployment, the Deployment webhook and YurtManager create a circular dependency. 

Solutions:
1. Change YurtManager deploying method, like static pod
2. YurtManager is in charge of managing the webhook, we can modify the internal implementation of YurtManager
3. Controller is responsible for both creating and updating  However, there will be a period of unavailability(wrong configuration information)
4. FailurePolicy set to ignore(difficult to detect in the case of malfunction)
##### Workflow of mutating webhook
1. If the intercepted Deployment's ownerReferences field is empty, filter it directly
2. Find the corresponding YurtAppConfigurationReplacement resource by ownerReferences, if not, filter directly
3. Find the replacements involved, get the corresponding configuration, and inject them into workloads
#### YurtAppConfigurationReplacement Validating Webhook
1. Verify that only one field of replacement is selected
2. verify that replicas and upgradeStrategy are selected only once
#### YurtAppConfigurationReplacement Controller
##### Task 1
1. Get update events by watching the YurtAppConfigurationReplacement resource
2. Trigger the Deployment mutating webhook by modifying an annotation or label
##### Task 2
1. Get delete events by watching the YurtAppConfigurationReplacement resource
2. Render the configuration according to the YurtAppSet workload template and the watching YurtAppConfigurationReplacement
### User Stories
#### Story 1
Use YurtAppSet with YurtAppConfigurationReplacement for personalized configuration of each region. Create YurtAppConfigurationReplacement first and then create YurtAppSet. If update is needed, modify YurtAppConfigurationReplacement resource directly. 
#### Story 2
Use YurtAppDaemon with YurtAppConfigurationReplacement for personalized configuration of each region. The usage is similar as YurtAppSet. 
## Implementation History
- [ ] : YurtAppConfigurationReplacement API CRD
- [ ] : Deployment Mutating Webhook
- [ ] : YurtAppConfigurationReplacement controller
- [ ] : Resolve circular dependency
- [ ] : YurtAppConfigurationReplacement validating webhook
