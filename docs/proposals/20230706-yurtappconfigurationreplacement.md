---
title: Proposal about YurtAppConfigurationReplacement
authors:
  - "@vie-serendipity"
  - "@rambohe-ch"
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
			- [Advanced features](#advanced-features)
		- [User Stories](#user-stories)
			- [Story 1 (General)](#story-1-general)
			- [Story 2 (Specific)](#story-2-specific)
			- [Story 3 (Specific)](#story-3-specific)
			- [Story 4 (Specific)](#story-4-specific)
	- [Implementation History](#implementation-history)

## Glossary
### YurtAppConfigurationReplacement
YurtAppConfigurationReplacement is a new CRD used to customize the configuration of the workloads managed by YurtAppSet/YurtAppDaemon. It provides an simple and straightforward way to configure every field of the workload under each nodepool. 
## Summary
Due to the objective existence of heterogeneous environments such as resource configurations and network topologies in each geographic region, the workload configuration is always different in each region. The workloads(Deployment/StatefulSet) of nodepools in different regions can be rendered through simple configuration by using YurtAppConfigurationReplacement which also supports multiple resources(YurtAppSet/YurtAppDaemon).
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

1. Considering the simplicity of customized rendering configuration, an incremental-like approach is used to implement injection, i.e., only the parts that need to be modified need to be declared. They are essentially either some existing resources, such as ConfigMap, Secret, etc., or some custom fields such as Replicas, Env, etc. Therefore, it is reasonable to abstract these configurable fields into an Item. The design of Item refers to the design of VolumeSource in kubernetes. 
2. In order to inject item into the workloads, we should create a new CRD named YurtAppConfigurationReplacemnet, which consist of replacements. Each replacement replace a set of configuration for matching nodepools. 
<img src = "../img/yurtappconfigurationreplacement/inspiration.png" width="600">

### YurtAppConfigurationReplacement API
1. YurtAppConfigurationReplacement needs to be bound to YurtAppSet/YurtAppDaemon. 
Considering that there are multiple Deployment/StatefulSet per nodepool, as shown below, it must be bound to YurtAppSet/YurtAppDaemon for injection. We use subject field to bind it to YurtAppSet/YurtAppDaemon. 
2. YurtAppConfigurationReplacement is responsible for injection of Replacement. We only need to create a new YurtAppConfigurationReplacement resource for all nodepools under a YurtAppSet/YurtAppDaemon resource. 

```go
// ImageItem specifies the corresponding container and the claimed image
type ImageItem struct {
	// ContainerName represents name of the container 
	// in which the Image will be replaced
	ContainerName string `json:"containerName"`
	// ImageClaim represents the claimed image name 
	// which is injected into the container above
	ImageClaim string `json:"imageClaim"`
}

// EnvItem specifies the corresponding container and the claimed env
type EnvItem struct {
	// ContainerName represents name of the container 
	// in which the env will be replaced
	ContainerName string `json:"containerName"`
	// EnvClaim represents the detailed enviroment variables container contains
	EnvClaim map[string]string `json:"envClaim"`
}

// PersistentVolumeClaimItem specifies the corresponding container and the claimed pvc
type PersistentVolumeClaimItem struct {
	// PVCSource represents pvcClaim name.
	PVCSource string `json:"pvcSource"`
	// PVCTarget represents the PVC corresponding to the volume above.
	// PVCTarget supprot advanced features like wildcard. 
	// By naming pvc as pvcName-{{nodepool}}, all pvc can be injected at once.
	PVCTarget string `json:"pvcTarget"`
}

// ConfigMapItem specifies the corresponding containerName and the claimed configMap
type ConfigMapItem struct {
	// ConfigMapSource represents configMap name
	ConfigMapSource string `json:"configMapSource"`
	// ConfigMapTarget represents the ConfigMap corresponding to the volume above. 
	// ConfigMapTarget supprot advanced features like wildcard. 
	// By naming configMap as configMapName-{{nodepool}}, all configMap can be injected at once.
	ConfigMapTarget string `json:"configMapTarget"`
}

type SecretItem struct {
	// SecretSource represents secret name.
	SecretSource string `json:"secretSource"`
	// SecretTarget represents the Secret corresponding to the volume above.
	// SecretTarget supprot advanced features like wildcard. 
	// By naming secret as secretName-{{nodepool}}, all secret can be injected at once.
	SecretTarget string `json:"secretTarget"`
}



// Item represents configuration to be injected.
// Only one of its members may be specified.
type Item struct {
	Image                 *ImageItem                 `json:"image"`
	ConfigMap             *ConfigMapItem             `json:"configMap"`
	Secret                *SecretItem                `json:"secret"`
	Env                   *EnvItem                   `json:"env"`
	PersistentVolumeClaim *PersistentVolumeClaimItem `json:"persistentVolumeClaim"`
	Replicas              *int                       `json:"replicas"`
	UpgradeStrategy       *string                    `json:"upgradeStrategy"`
}
// Replacement describe a set of nodepools and their shared or identical configurations
type Replacement struct {
	Pools []string `json:"pools"`
	Items []Item   `json:"items"`
}

type Subject struct {
	metav1.TypeMeta `json:",inline"`
	// NameSpace is the namespace of YurtAppSet or YurtAppDaemon
	NameSpace string `json:"nameSpace"`
	// Name is the name of YurtAppSet or YurtAppDaemon
	Name      string `json:"name"`
}

type YurtAppConfigurationReplacement struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Describe the object this replacement belongs
	Subject Subject `json:"subject"`

	// Describe detailed multi-region configuration of the subject above
	Replacements []Replacement `json:"replacements"`
}
```
### Architecture
The whole architecture is shown below. 
<img src = "../img/yurtappconfigurationreplacement/architecture.png" width="800">

### Implementation Details
#### Deployment Mutating Webhook
##### Prerequisites for webhook (Resolving circular dependency)
Since YurtManager is deployed as a Deployment, the Deployment webhook and YurtManager create a circular dependency. 

Solutions:
1. Change YurtManager deploying method, like static pod
2. Controller is responsible for both creating and updating. However, there will be a period of unavailability(wrong configuration information)
3. Webhook's failurePolicy set to ignore(difficult to detect in the case of malfunction)
4. YurtManager is in charge of managing the webhook, we can modify the internal implementation of YurtManager(Recommended)
##### Workflow of mutating webhook
1. If the intercepted Deployment's ownerReferences field is empty, filter it directly
2. Find the corresponding YurtAppConfigurationReplacement resource by ownerReferences, if not, filter directly
3. Find the replacements involved, get the corresponding configuration, and inject them into workloads. 

Attention Points:
1. Note that injection is implemented by recalculating the final configuration according to the YurtAppSet workload template and the watching YurtAppConfigurationReplacement
2. The latter configuration always relpace the former. So the last configuration will really work
#### YurtAppConfigurationReplacement Validating Webhook
1. Verify that only one field of item is selected
2. Verify that replicas and upgradeStrategy are selected only once in a replacement
#### YurtAppConfigurationReplacement Controller
##### Task 1
1. Get update events by watching the YurtAppConfigurationReplacement resource
2. Trigger the Deployment mutating webhook by modifying an annotation or label
##### Task 2
1. Get delete events(delete members of pools) by watching the YurtAppConfigurationReplacement resource
2. Render the configuration according to the YurtAppSet workload template and the watching YurtAppConfigurationReplacement
#### Advanced features
Here is a scenario. Ten nodepools only need to be configured differently for configMap, it is not user-friendly to write ten replacements by using current scheme. 
We add a new feature like wildcard to simplify the usage. We can name all the configMap as configMap-{{nodepool}} so that we can inject the configMap into nodepool in the suffix. 
```yaml
replacements:
- pools:
    *
  items:
  - configMap:
      configMapSource: configMap-demo
      configMapTarget: configMapName-{{nodepool}}
```
In this way, we only need to write one replacement. Secret and pvc are similar. Through this feature, it will be easier to customize multi-region configuration.
### User Stories
#### Story 1 (General)
Use YurtAppSet with YurtAppConfigurationReplacement for customized configuration of each region. Create YurtAppConfigurationReplacement first and then create YurtAppSet. If update is needed, modify YurtAppConfigurationReplacement resource directly. For YurtAppDaemon, the usage is similar. 
#### Story 2 (Specific)
For example, if there are three locations, Beijing and Hangzhou have the similar configuration, and Shanghai is not the same. They have different image version, replicas, and configMap. We can configure it as follows:
```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtAppConfigurationReplacement
metadata:
  name: demo1
subject:
  apiVersion: apps.openyurt.io/v1alpha1
  kind: YurtAppSet
  nameSpace: default
  name: yurtappset-demo
replacements:
- pools:
    beijing
	hangzhou
  items:
  - image:
      containerName: nginx
	  imageClaim: nginx:1.14.2
  - configMap:
      configMapSource: configMap-demo1
      configMapTarget: configMap-demo2
  - replicas: 3
- pools:
    shanghai
  items:
  - image:
      containerName: nginx
	  imageClaim: nginx:1.13.2
  - configMap:
      configMapSource: configMap-demo1
      configMapTarget: configMap-demo3
  - replicas: 5
```
#### Story 3 (Specific)
If all nodepools differ only in configMap, we can configure as follows:
```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtAppConfigurationReplacement
metadata:
  name: demo1
subject:
  apiVersion: apps.openyurt.io/v1alpha1
  kind: YurtAppSet
  nameSpace: default
  name: yurtappset-demo
replacements:
- pools:
    *
  items:
  - configMap:
      configMapSource: configMap-demo1
      configMapTarget: prefixName-{{nodepool}}
```
#### Story 4 (Specific)
Beijing and Hangzhou have most of the same configuration, only the replicas are different. We can configure their identical parts first. Because in our configuration resource, the later configuration will always replace the former. So we can add different configuration of beijing to the end. 
```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtAppConfigurationReplacement
metadata:
  name: demo1
subject:
  apiVersion: apps.openyurt.io/v1alpha1
  kind: YurtAppSet
  nameSpace: default
  name: yurtappset-demo
replacements:
- pools:
    beijing
	hangzhou
  items:
  - image:
      containerName: nginx
	  imageClaim: nginx:1.14.2
  - configMap:
      configMapSource: configMap-demo1
      configMapTarget: configMap-demo2
  - replicas: 3
- pools:
    beijing
  items:
  - replicas: 5
```
## Implementation History
- [ ] : YurtAppConfigurationReplacement API CRD
- [ ] : Deployment Mutating Webhook
- [ ] : YurtAppConfigurationReplacement controller
- [ ] : Resolve circular dependency
- [ ] : YurtAppConfigurationReplacement validating webhook
