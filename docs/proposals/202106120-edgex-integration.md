---
title: EdgeX integration with OpenYurt
authors:
  - '@yixingjia'
  - '@lwmqwer'
reviewers:
  - '@kadisi'
  - '@Fei-Guo'
  - '@rambohe-ch'
creation-date: 2021-06-12T00:00:00.000Z
last-updated: 2021-06-12T00:00:00.000Z
status: provisional
---

# EdgeX integration with OpenYurt

## Table of Contents

- [EdgeX integration with OpenYurt](#edgex-integration-with-openyurt)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Overview](#overview)
      - [1. EdgeX Customer Resource Definition](#1-edgex-customer-resource-definition)
      - [2. EdgeX Controller](#2-edgex-controller)
        - [EdgeX CR creation](#edgex-cr-creation)
        - [EdgeX CR status update](#edgex-cr-status-update)
        - [EdgeX CR deletion](#edgex-cr-deletion)
  - [Update/Upgrade](#updateupgrade)
  - [Implementation History](#implementation-history)

## Glossary

Refer to the [OpenYurt Glossary](https://github.com/openyurtio/openyurt/blob/master/docs/proposals/00_openyurt-glossary.md) and the [EdgeX Foundry Glossary](https://docs.edgexfoundry.org/1.3/general/Definitions/).

## Summary

In previous proposal [edge-device-management](20210310-edge-device-management.md) we define the object mappings between EdgeX Foundry and OpenYurt. In this proposal we will cover the EdgeX Foundry management part.
EdgeX Foundry itself is a loosely coupled, micro-services based framework. The deployment topology can be pretty flexible base on the usage scenarios. The plugin based design enable user easily create their own micro-services and connected to EdgeX Foundry to meet their specific requirements. In this proposal we want make it easy to run EdgeX on Edge side and meanwhile keep the capability of run any customized components user specified together with EdgeX Foundry.

### Goals

- Design a new CRD EdgeX to represent the EdgeX instance.
- To support EdgeX services auto deployment when a new EdgeX CR(Customer Resource) created.
- Implement a new EdgeX controller to manage EdgeX CR and mapping to United Deployment. Leverage the United Deployment's ability of deploy EdgeX on node pool/cross sites scenario.
- To support run user defined components together with EdgeX.

### Non-Goals/Future Work

Non-goals are limited to the scope of this proposal. These features may evolve in the future.

- Compatible check for the optional/user defined components. For the optional components provided by EdgeX community it will works as it designed since those components have past the compatible test. For any user created components, user should be responsible for the compatibility testing.

## Proposal

### User Stories

1. As an end-user, I would like to deploy EdgeX to the Edge node
2. As an end-user, I would like to deploy my container based application to the Edge node and connect to the EdgeX I deployed.
3. As an end-user, I would like to update the EdgeX (add/remove one or more optional components).

### Overview

We define a new CR EdgeX to represent the EdgeX instance running on the Edge side, the relation between the EdgeX CR and EdgeX instance is 1:1. When we have multiple EdgeX instances running on the Edge side, the same number of EdgeX CRs will be created. For each EdgeX instance it may contains different micro-service components, each of the components will be live in the form of a Kubernetes deployment. For the deployments that compose a EdgeX instance it can basically grouped into two categories:
- Required deployments : For the deployments in this group are mandatory and will be deployed with each EdgeX instance.
- Optional deployments: For deployments in this group is optional can user can specify which deployments they want to deploy.

Consider it is quite common to have multiple EdgeX instance in a single Kubernetes Cluster, With traditional service+deployment solutions, for the components in each EdgeX instance it require a unique name. When the EdgeX instance number increased the name management will be a critical issue need to address. In this proposal we leverage the United Deployment feature of OpenYurt. In that way all the EdgeX Instance share the same service name which can avoid the service name explosion.

![Deployment Topology](../img/edgex/overview.png)

As the deployment topology diagram shows.  Each components in the EdgeX instance will have a corresponding service mapping to it. The service will mapping to a United Deployment instance(In the backend it is actually mapping to all the deployments inside the united deployment) On the edge side a deployment with a unique name will be created and registered in the service's endpoints. Service topology policy will be automatically applied by United Deployment controller to make sure the traffic will be limited to Edge/Node Pool level.From one side we can make the management easier and on the other side we can still keep the EdgeX instance isolation.
#### 1. EdgeX Customer Resource Definition

Following is the definition of `EdgeX` CRD and its related Golang struct. The CRD will be created when the EdgeX controller is up and running.
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.4.1
  creationTimestamp: null
  name: edgexes.device.openyurt.io
spec:
  group: device.openyurt.io
  names:
    kind: EdgeX
    listKind: EdgeXList
    plural: edgexes
    singular: edgex
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: EdgeX is the Schema for the edgexes API
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: EdgeXSpec defines the desired state of EdgeX
            properties:
              additinalcomponents:
                items:
                  properties:
                    deploymentspec:
                      description: DeploymentTemplateSpec defines the pool template
                        of Deployment.
                      properties:
                        metadata:
                          type: object
                        spec:
                          description: DeploymentSpec is the specification of the
                            desired behavior of the Deployment.
                          type: object
                      required:
                      - spec
                      type: object
                    servicespec:
                      description: DeploymentTemplateSpec defines the pool template
                        of Deployment.
                      properties:
                        metadata:
                          type: object
                        spec:
                          description: ServiceSpec describes the attributes that a
                            user creates on a service.
                          type: object
                      required:
                      - spec
                      type: object
                  type: object
                type: array
              poolname:
                type: string
              version:
                type: string
            type: object
          status:
            description: EdgeXStatus defines the observed state of EdgeX
            properties:
              componentstatus:
                description: ComponentStatus is the status of edgex component.
                items:
                  properties:
                    deploymentstatus:
                      description: DeploymentStatus is the most recently observed
                        status of the Deployment.
                      type: object
                    name:
                      type: string
                  type: object
                type: array
              conditions:
                description: Current Edgex state
                type: array
                x-kubernetes-list-map-keys:
                - type
                x-kubernetes-list-type: map
              initialized:
                type: boolean
              observedGeneration:
                description: ObservedGeneration is the most recent generation observed
                  for this UnitedDeployment. It corresponds to the UnitedDeployment's
                  generation, which is updated on mutation by the API Server.
                format: int64
                type: integer
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
```

```go
// DeploymentTemplateSpec defines the pool template of Deployment.
type DeploymentTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              appsv1.DeploymentSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the pool template of Deployment.
type ServiceTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              corev1.ServiceSpec `json:"spec"`
}

type ComponentSpec struct {
	// +optional
	Deployment DeploymentTemplateSpec `json:"deploymentspec,omitempty"`

	// +optional
	Service ServiceTemplateSpec `json:"servicespec,omitempty"`
}

// EdgeXSpec defines the desired state of EdgeX
type EdgeXSpec struct {
	Version string `json:"version,omitempty"`

	PoolName string `json:"poolname,omitempty"`
	// +optional
	AdditionalComponents []ComponentSpec `json:"additinalcomponents,omitempty"`
}

type ComponentStatus struct {
	Name string `json:"name,omitempty"`

	Deployment appsv1.DeploymentStatus `json:"deploymentstatus,omitempty"`
}

// EdgeXStatus defines the observed state of EdgeX
type EdgeXStatus struct {
	// ObservedGeneration is the most recent generation observed for this UnitedDeployment. It corresponds to the
	// UnitedDeployment's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Initialized bool `json:"initialized,omitempty"`

	// ComponentStatus is the status of edgex component.
	// +optional
	ComponentStatus []ComponentStatus `json:"componentstatus,omitempty"`
	// Current Edgex state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=edgexes

// EdgeX is the Schema for the edgexes API
type EdgeX struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeXSpec   `json:"spec,omitempty"`
	Status EdgeXStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EdgeXList contains a list of EdgeX
type EdgeXList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EdgeX `json:"items"`
}
```
When user create an EdgeX CR, the EdgeX version field is required. For each specific version, the required components section are fixed and will be created automatically by the EdgeX controller. If user want run any optional components or components created by themselves, those components must be declared explicitly in the optionalComponents section.

![EdgeX Instance](../img/edgex/edgexinstance.png)
As the EdgeX instance diagram shows, the following 8 Components are required for EdgeX Hanoi version
- Core Data service: The core data micro service provides centralized persistence for data collected by devices.
- Command service: The command service enables the issuance of commands or actions to devices on behalf of other micro services/applications/external systems that need to modify the settings of a collection of devices.
- Metadata service: The metadata service has the knowledge about the devices and sensors and how to communicate with them used by the other services.
- Registry & Config service: It provides other EdgeX Foundry micro services with information about associated services within the system and micro services configuration properties
- Consul: EdgeX Foundry's reference implementation configuration and registry service.
- Redis: EdgeX Foundry's reference implementation database (for sensor data, metadata and all things that need to be persisted in a database) is Redis
- Support-notification: When another system or a person needs to know that something occurred in EdgeX, the alerts and notifications micro service sends that notification.
- support-scheduler: Scheduler micro service provide an internal EdgeX “clock” that can kick off operations in any EdgeX service.

All other components are optional include user created components.

#### 2. EdgeX Controller

EdgeX controller is responsible for the EdgeX CR lifecycle management.
##### EdgeX CR creation
- User create EdgeX CR
- EdgeX controller check the EdgeX version specified in the EdgeX CR
- For each required components, If the related united deployment instance does not exist create a new united deployment. Add a new pool section in the united deployment。Use the predefined service name to check the service, if not exist, create one.
- For each optional components, The controller will do the similar jobs like the required components. If the user define a service for the optional component then the controller will create a service with the name specified by the user. If the service name already exist, it will use the existing service.

##### EdgeX CR status update
- EdgeX controller will periodically check the Deployments belong to the EdgeX CR and update the CR status. Please note in this version we only deploy EdgeX controller in the cloud. So when the network during the network outage , the status will not updated.

##### EdgeX CR deletion
When EdgeX CR is deleted the follow actions will performed by the controller.
- For each components(both required and optional), the mapping deployments will be deleted from the Edge pool/Edge node by update the united deployment.
- Check the service, if this is the last deployment in the service, then the service should also deleted.
- Check the united deployment, if the united deployment don't have any pool object, then delete the united deployment(TO be discussed)

## Update/Upgrade
Follow the Kubernetes application update strategy. User can update/upgrade the EdgeX CR by apply yaml file update. But please note that the compatibility between different EdgeX version id defined in the EdgeX release document. Please read the release note before take any upgrade actions.

## Implementation History

