---
title: Edge Device Management
authors:
  - '@charleszheng44'
reviewers:
  - '@yixingjia'
  - '@Fei-Guo'
  - '@rambohe-ch'
  - '@kadisi'
  - '@huangyuqi'
  - '@Walnux'
creation-date: 2021-03-10T00:00:00.000Z
last-updated: 2021-03-10T00:00:00.000Z
status: provisional
---
# Managing Edge Devices using EdgeX Foundry

## Table of Contents

- [Managing Edge Devices using EdgeX Foundry](#managing-edge-devices-using-edgex-foundry)
  * [Table of Contents](#table-of-contents)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
    + [Goals](#goals)
    + [Non-Goals/Future Work](#non-goalsfuture-work)
  * [Proposal](#proposal)
    + [User Stories](#user-stories)
    + [Components](#components)
      - [1. DeviceProfile](#1-deviceprofile)
      - [2. Device](#2-device)
      - [3. DeviceService](#3-deviceservice)
    + [Interaction with EdgeX Foundry](#interaction-with-edgex-foundry)
      - [1. Setting up EdgeX Foundry](#1-setting-up-edgex-foundry)
      - [2. CRUD objects on EdgeX Foundry](#2-crud-objects-on-edgex-foundry)
      - [3. Controlling the device declaratively](#3-controlling-the-device-declaratively)
    + [System Architecture](#system-architecture)
  * [Upgrade Strategy](#upgrade-strategy)
  * [Implementation History](#implementation-history)

## Glossary

Refer to the [OpenYurt Glossary](docs/proposals/00_openyurt-glossary.md) and the [EdgeX Foundry Glossary](https://docs.edgexfoundry.org/1.3/general/Definitions/).

## Summary

This proposal introduces an approach to managing IoT devices on OpenYurt using Kubernetes custom resources and EdgeX Foundry. This approach leverages EdgeX Foundry's ability in IoT devices management and uses Kubernetes custom resources to abstract edge devices. As the world's first pluggable IoT platform, EdgeX Foundry has been adopted by most edge device vendors since it was open-sourced. Therefore, the proposed approach inherently supports a rich spectrum of edge devices. On the other hand, we define several custom resource definitions(CRD) that acts as the mediator between OpenYurt and EdgeX Foundry. These CRDs allow users to manage edge devices in a declarative way, which provides users with a Kubernetes-native experience.

## Motivation

Extending the Kubernetes to the edge is not a new topic. However, to support all varieties of edge devices, existing Kubernetes-based edge frameworks have to develop dedicated adaptors for each category of the device from scratch and change the original Kubernetes architecture significantly, which entails great development efforts with the loss of some upstream K8S features. Instead, we are inspired by the Unix philosophy, i.e., “Do one thing, and do it well,” and believe that a mature edge IoT platform should do the device management. To that end, we integrate EdgeX into the OpenYurt, which supports a full spectrum of devices and supports all upstream K8S features.

### Goals

- To design a new custom resource definition(CRD), DeviceProfile, to represent different categories of devices
- To design a new CRD, Device, that represents physical edge devices
- To design a new CRD, DeviceService, that defines the way of how to connect to a specific device
- To support device management using DeviceProfile, Device, DeviceService, and EdgeX Foundry 
- To support automatically setting up of the EdgeX Foundry on the OpenYurt
- To support declarative device state modification, i.e., modifying the device's properties by changing fields of the device CRs

### Non-Goals/Future Work

Non-goals are limited to the scope of this proposal. These features may evolve in the future.

- To implement DeviceService for any specific protocol
- To support data transmission between edge devices and external services
- To support edge data processing   

## Proposal

### User Stories

1. As a vendor, I would like to connect a category of devices into OpenYurt.
2. As an end-user, I would like to define how to connect a device, which belongs to a supported DeviceProfile, into the OpenYurt.
3. As an end-user, I would like to connect a new device, which belongs to a supported DeviceProfile, to the OpenYurt.
4. As an end-user, I would like to modify the states of devices by changing the values of device properties defined in corresponding Device CRs.
5. As an end-user, I would like to disconnect a device by deleting the corresponding Device CR.

### Components

Since we leverage EdgeX Foundry to manage edge devices, we define three new custom resource definitions(CRD) corresponding to the three core services of the EdgeX Foundry. They are [DeviceProfile](#1-deviceprofile), [Device](#3-device), and [DeviceService](#3-deviceservice). The `DeviceProfile` defines a type of device belonging to a certain protocol, which defines some generic information like the manufacturer's name, the device description, and the device model. DeviceProfile also defines what kind of resources (e.g., temperature, humidity) this type of device provided and how to read/write these resources. The `DeviceService` includes how to connect a device, like the URL of the device. The `DeviceService` can not exist alone. Every `DeviceService` must associate with a `DeviceProfile`. The `Device` gives the detailed definition of a specific device, like which `DeviceProfile` belongs to and which `DeviceService` it used to connect to the system. For a more detailed description of the three objects, please refer to the [EdgeX Foundry's documentation](https://docs.edgexfoundry.org/1.2/microservices/device/profile/Ch-DeviceProfile/). The relationship between these CRDs and related structs is shown below.

![struct-relation](../img/struct-relation.png)

#### 1. DeviceProfile

Followings are definitions of `DeviceProfile` CRD and its related Golang structs. 

```go
type DeviceProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceProfileSpec   `json:"spec,omitempty"`
	Status DeviceProfileStatus `json:"status,omitempty"`
}

// DeviceProfileSpec defines the desired state of DeviceProfile
type DeviceProfileSpec struct {
	// Description of the Device
	Description string `json:"description,omitempty"`
	// Manufacturer of the device
	Manufacturer string `json:"manufacturer,omitempty"`
	// Model of the device
	Model string `json:"model,omitempty"`
	// EdgeXLabels used to search for groups of profiles on EdgeX Foundry
	EdgeXLabels     []string         `json:"labels,omitempty"`
	// Available DeviceResources of the profile
	DeviceResources []DeviceResource `json:"deviceResources,omitempty"`
	// Available DeviceCommands of the profile
	DeviceCommands []ProfileResource `json:"deviceCommands,omitempty"`
	// Available CoreCommands of the profile
	CoreCommands   []Command         `json:"coreCommands,omitempty"`
}

// DeviceProfileStatus defines the observed state of DeviceProfile
type DeviceProfileStatus struct {
	// EdgeXId is the Id assigned by the EdgeX Foundry
	EdgeXId      string `json:"id,omitempty"`
	// AddedToEdgeX indicates if the DeviceProfile has been added to the EdgeX Foundry
	AddedToEdgeX bool   `json:"addedToEdgeX,omitempty"`
}

// DeviceResource is the resource/data that 
type DeviceResource struct {
	Description string            `json:"description"`
	Name        string            `json:"name"`
	Tag         string            `json:"tag,omitempty"`
	Properties  ProfileProperty   `json:"properties"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// ProfileProperty defines the property details of the DeviceResource
type ProfileProperty struct {
	Value PropertyValue `json:"value"`
	Units Units         `json:"units,omitempty"`
}

// PropertyValue defines the value format of the property
type PropertyValue struct {
    // ValueDescriptor Type of property after transformations
	Type         string `json:"type,omitempty"`         
    // Read/Write Permissions set for this property
	ReadWrite    string `json:"readWrite,omitempty"`    
    // Minimum value that can be get/set from this property
	Minimum      string `json:"minimum,omitempty"`      
    // Maximum value that can be get/set from this property
	Maximum      string `json:"maximum,omitempty"`      
    // Default value set to this property if no argument is passed
	DefaultValue string `json:"defaultValue,omitempty"` 
    // Size of this property in its type  (i.e. bytes for numeric types, characters for string types)
	Size         string `json:"size,omitempty"`         
    // Mask to be applied prior to get/set of property
	Mask         string `json:"mask,omitempty"`         
    // Shift to be applied after masking, prior to get/set of property
	Shift        string `json:"shift,omitempty"`        
    // Multiplicative factor to be applied after shifting, prior to get/set of property
	Scale        string `json:"scale,omitempty"`        
    // Additive factor to be applied after multiplying, prior to get/set of property
	Offset       string `json:"offset,omitempty"`       
    // Base for property to be applied to, leave 0 for no power operation (i.e. base ^ property: 2 ^ 10)
	Base         string `json:"base,omitempty"`         
	// Required value of the property, set for checking error state. Failing an
	// assertion condition will mark the device with an error state
	Assertion     string `json:"assertion,omitempty"`
	Precision     string `json:"precision,omitempty"`
    // FloatEncoding indicates the representation of floating value of reading. It should be 'Base64' or 'eNotation'
	FloatEncoding string `json:"floatEncoding,omitempty"` 
	MediaType     string `json:"mediaType,omitempty"`
}

// Units defines the unit of the property
type Units struct {
	Type         string `json:"type,omitempty"`
	ReadWrite    string `json:"readWrite,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

// ProfileResource includes the available operations to the DeviceResource 
type ProfileResource struct {
	Name string              `json:"name,omitempty"`
	Get  []ResourceOperation `json:"get,omitempty"`
	Set  []ResourceOperation `json:"set,omitempty"`
}

// ResourceOperation gives the details of how to Get/Set the DeviceResource
type ResourceOperation struct {
	Index     string `json:"index,omitempty"`
	Operation string `json:"operation,omitempty"`
	// Deprecated
	Object string `json:"object,omitempty"`
	// The replacement of Object field
	DeviceResource string `json:"deviceResource,omitempty"`
	Parameter      string `json:"parameter,omitempty"`
	// Deprecated
	Resource string `json:"resource,omitempty"`
	// The replacement of Resource field
	DeviceCommand string            `json:"deviceCommand,omitempty"`
	Secondary     []string          `json:"secondary,omitempty"`
	Mappings      map[string]string `json:"mappings,omitempty"`
}

// Command defines the available commands for end users to control the devices
// NOTE: a Command usually corresponding to a DeviceCommand and a DeviceResource
type Command struct {
	// EdgeXId is a unique identifier used by EdgeX Foundry, such as a UUID
	EdgeXId string `json:"id,omitempty"`
	// Command name (unique on the profile)
	Name string `json:"name,omitempty"`
	// Get Command
	Get Get `json:"get,omitempty"`
	// Put Command
	Put Put `json:"put,omitempty"`
}

// Put defines the details of the Put operation
type Put struct {
	Action         `json:",inline"`
	ParameterNames []string `json:"parameterNames,omitempty"`
}

// Get defines the details of the Get operation
type Get struct {
	Action `json:",omitempty"`
}

// Action defins the details of a HTTP operation
type Action struct {
	// Path used by service for action on a device or sensor
	Path string `json:"path,omitempty"`
	// Responses from get or put requests to service
	Responses []Response `json:"responses,omitempty"`
	// Url for requests from command service
	URL string `json:"url,omitempty"`
}

// Response of a Get or Put request to a service
type Response struct {
	Code           string   `json:"code,omitempty"`
	Description    string   `json:"description,omitempty"`
	ExpectedValues []string `json:"expectedValues,omitempty"`
}

```
#### 2. Device

Followings are the definition of the `Device` CRD and its related Golang structs:

```go
// Device is the Schema for the devices API
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSpec   `json:"spec,omitempty"`
	Status DeviceStatus `json:"status,omitempty"`
}

// DeviceSpec defines the desired state of Device
type DeviceSpec struct {
	Description string `json:"description,omitempty"`
	// Admin state (locked/unlocked)
	AdminState AdminState `json:"adminState,omitempty"`
	// Operating state (enabled/disabled)
	OperatingState OperatingState `json:"operatingState,omitempty"`
	// A map of supported protocols for the given device
	Protocols map[string]ProtocolProperties `json:"protocols,omitempty"`
	// Other labels applied to the device to help with searching
	Labels []string `json:"labels,omitempty"`
	// Device service specific location (interface{} is an empty interface so
	// it can be anything)
	Location string `json:"location,omitempty"`
	// Associated Device Service - One per device
	Service string `json:"service"`
	// Associated Device Profile - Describes the device
	Profile string `json:"profile"`
	// TODO support the following field
	// A list of auto-generated events coming from the device
	// AutoEvents     []AutoEvent                   `json:"autoEvents"`
	DeviceProperties map[string]DesiredPropertyState `json:"deviceProperties,omitempty"`
}

type DesiredPropertyState struct {
	Name         string `json:"name"`
	PutURL       string `json:"putURL,omitempty"`
	DesiredValue string `json:"desiredValue"`
}

type ActualPropertyState struct {
	Name        string `json:"name"`
	GetURL      string `json:"getURL,omitempty"`
	ActualValue string `json:"actualValue"`
}

// DeviceStatus defines the observed state of Device
type DeviceStatus struct {
	// Time (milliseconds) that the device last provided any feedback or
	// responded to any request
	LastConnected int64 `json:"lastConnected,omitempty"`
	// Time (milliseconds) that the device reported data to the core
	// microservice
	LastReported int64 `json:"lastReported,omitempty"`
	// AddedToEdgeX indicates whether the object has been successfully
	// created on EdgeX Foundry
	AddedToEdgeX     bool                           `json:"addedToEdgeX,omitempty"`
	DeviceProperties map[string]ActualPropertyState `json:"deviceProperties,omitempty"`
	Id               string                         `json:"id,omitempty"`
}

type AdminState string

const (
	Locked   AdminState = "LOCKED"
	UnLocked AdminState = "UNLOCKED"
)

type OperatingState string

const (
	Enabled  OperatingState = "ENABLED"
	Disabled OperatingState = "DISABLED"
)

type ProtocolProperties map[string]string

```

#### 3. DeviceService

Followings are definitions of `DeviceService` CRD and related Golang structs

```go
// DeviceService is the Schema for the deviceservices API
type DeviceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceServiceSpec   `json:"spec,omitempty"`
	Status DeviceServiceStatus `json:"status,omitempty"`
}

// DeviceServiceSpec defines the desired state of DeviceService
type DeviceServiceSpec struct {
	Description string `json:"description,omitempty"`
	// EdgeXLabels are used by EdgeX Foundry for locating/grouping device services
	EdgeXLabels []string `json:"labels,omitempty"`
	// address (MQTT topic, HTTP address, serial bus, etc.) for reaching the service
	Addressable Addressable `json:"addressable,omitempty"`
}

// DeviceServiceStatus defines the observed state of DeviceService
type DeviceServiceStatus struct {
    // the Id assigned by the EdgeX foundry
	EdgeXId string `json:"id,omitempty"`
	AddedToEdgeX bool `json:"addedToEdgeX,omitempty"`
    LastConnected int64 `json:"lastConnected,omitempty"`
	// time in milliseconds that the device last reported data to the core
	LastReported int64 `json:"lastReported,omitempty"`
    // operational state - either enabled or disabled
	OperatingState OperatingState `json:"operatingState,omitempty"`
    // Device Service Admin State
	AdminState AdminState `json:"adminState,omitempty"`
}

type Addressable struct {
	// ID is a unique identifier for the Addressable, such as a UUID
	Id string `json:"id,omitempty"`
	// Name is a unique name given to the Addressable
	Name string `json:"name,omitempty"`
	// Protocol for the address (HTTP/TCP)
	Protocol string `json:"protocol,omitempty"`
	// Method for connecting (i.e. POST)
	HTTPMethod string `json:"method,omitempty"`
	// Address of the addressable
	Address string `json:"address,omitempty"`
	// Port for the address
	Port int `json:"port,omitempty"`
	// Path for callbacks
	Path string `json:"path,omitempty"`
	// For message bus protocols
	Publisher string `json:"publisher,omitempty"`
	// User id for authentication
	User string `json:"user,omitempty"`
	// Password of the user for authentication for the addressable
	Password string `json:"password,omitempty"`
	// Topic for message bus addressables
	Topic string `json:"topic,omitempty"`
}
```

### Interaction with EdgeX Foundry 

#### 1. Setting up EdgeX Foundry

As OpenYurt supports all upstream Kubernetes features, the EdgeX Foundry deployment should be the same as deploying it on the Kubernetes. The Deployment yaml files can be found in the [EdgeX Foundry example repository](https://github.com/edgexfoundry/edgex-examples/tree/master/deployment/kubernetes), by simply applying them, the necessary services will be deployed. Also, we will allow users to choose if they want to set up the EdgeX Foundry automatically when converting the Kubernetes cluster to the OpenYurt cluster using the `yurtctl` command-line tool. 

#### 2. CRUD objects on EdgeX Foundry

To conciliate the device states between OpenYurt and EdgeX Foundry, a new component, DeviceManager, will be installed on the OpenYurt. The DeviceManager includes three controllers, i.e., DeviceProfile Controller, DeviceService Controller, and the Device Controller, which act as mediators between the OpenYurt and the EdgeX Foundry and are responsible for reconciling the states of the device-related custom resources with the states of the corresponding objects on the EdgeX Foundry.

Following is the process of connecting a new device to the OpenYurt through EdgeX Foundry.

![connect-device](../img/connect-device.png)

The vendor applies the DeviceProfile CR.
2. The DeviceManager creates the related ValueDescriptor on the EdgeX MetaData Service.
3. The DeviceManager create the DeviceProfile on the EdgeX CoreData Service.
4. The end-user applies the Device CR.
5. The DeviceManager creates the Device object on the EdgeX CoreData Service.
6. The Device supported commands are created on the EdgeX Command Service.
7. The end-user applies the DeviceService CR.
8. The DeviceManager creates the DeviceService object on the EdgeX CoreData Service.
9. The Physical Device is connected to the OpenYurt through the device service instance (The actual microservice that talks to the physical devices depend on what protocol the device uses, the device service instance may be deployed as a pod on edge nodes or be deployed outside of the OpenYurt cluster).

#### 3. Controlling the device declaratively

The declarative model is one of the core concepts of Kubernetes. This model allows users to declare the desired states of resources/workloads, and the corresponding controllers/operators will reconcile the actual states with the desired states. We will add a new field, `DeviceProperties`, to the `Device` CRD, which will hold the device properties that can be modified declaratively. The general idea behind the `DeviceProperties` is that the vendor (or anyone who defines the `Device` CRD) will decide which `DeviceResource` can be changed declaratively and create a matching entry in the `DeviceProperties`. After the `Device` CR is applied to the cluster, the device controller will query the EdgeX Foundry command service, fetch the rest endpoint, and update the `DeviceProperties` entries. 

For example, following is a `DeviceProfile` CR represents a type of sensor devices

```yaml
apiVersion: device.openyurt.io/v1alpha1
kind: DeviceProfile
metadata:
  name: color-sensor 
spec:
  ...
  deviceResources:
    - name: lightcolor
      description: "JSON message"
      properties:
        value:
          { type: "String", readWrite: "W" , mediaType : "application/json" }
        units:
          { type: "String", readWrite: "R" }
  ...
  coreCommands:
    - name: lightcolor
      get:
        path: "/api/v1/device/{deviceId}/color"
        responses:
        -
          code: "200"
          description: "get current light color"
          expectedValues: ["color"]
        -
          code: "503"
          description: "service unavailable"
          expectedValues: []
      put:
        path: "/api/v1/device/{deviceId}/changeColor"
        responses:
        -
          code: "201"
          description: "set the light color"
        -
          code: "503"
          description: "service unavailable"
          expectedValues: []
```

The `DeviceProfile` contains one `deviceResources`, i.e., `lightcolor`, which supports both "read" and "write" operations. The profile also defines a corresponding `coreCommand`, which contains the REST API of the `lightcolor`. After this `DeviceProfile` is applied to the cluster, the following core command will be created on the EdgeX Foundry,

```json
{
  ... 
  "commands": [
    {
      "created": <created-timestamp>,
      "modified": <modifed-timestamp>,
      "id": "<command-id>",
      "name": "lightcolor",
      "get": {
        "path": "/api/v1/device/{deviceId}/color",
        "responses": [
          {
            "code": "200",
            "description": "get current light color",
            "expectedValues": [
              "color"
            ]
          },
          {
            "code": "503",
            "description": "service unavailable"
          }
        ],
        "url": "http://edgex-core-command:48082/api/v1/device/<device-id>/command/<set-command-id>"
      },
      "put": {
        "path": "/api/v1/device/{deviceId}/changeColor",
        "responses": [
          {
            "code": "201",
            "description": "set the light color"
          },
          {
            "code": "503",
            "description": "service unavailable"
          }
        ],
        "url": "http://edgex-core-command:48082/api/v1/device/<device-id>/command/<put-command-id>"
      }
    }
  ]
}
```

Then we can create a `Device` associated to the `DeviceProfile` as following,

```yaml
apiVersion: device.openyurt.io/v1alpha1
kind: Device 
metadata:
  name: testsensor
spec:
  ... 
  profile: color-sensor
  deviceProperties:
    lightcolor:
      name: lightcolor
      desiredValue: green
```

Assume that the initial light color is blue, then after applying the `Device`, the device controller will try to fetch the corresponding get/put URLs from the core command service, update the `testsensor.Spec.DeviceProperties[lightcolor]` `testsensor.Status.DeviceProperties[lightcolor]`, and then set the light color to green, the updated `Device` will look like the following,
```yaml
apiVersion: device.openyurt.io/v1alpha1
kind: Device 
metadata:
  name: testsensor
spec:
  ... 
  profile: color-sensor
  deviceProperties:
    lightcolor:
      name: lightcolor
      setURL: http://edgex-core-command:48082/api/v1/device/<device-id>/command/<put-command-id>
      desiredValue: green
status:
  deviceProperties:
    lightcolor:
      name: lightcolor
      getURL: http://edgex-core-command:48082/api/v1/device/<device-id>/command/<set-command-id>
      desiredValue: green
```

### System Architecture

Edge devices are usually located in the network regions that are unreachable from the cloud, while `DeviceService` needs to talk to edge devices directly. Therefore, we need to deploy the `DeviceService` on the edge node that connects to edge devices. On the other hand, when modifying the device properties, the device controller will need to send a request to the `DeviceService`. Hence, we will deploy a replica of the device controller in each network region. Specifically, the device controller will connect to the YurtHub, and the YurtHub will only pull the device CRDs related to connected edge devices. The system architecture is shown below,

![system-architecture](../img/system-arch.png)

## Upgrade Strategy

In the first implementation, we will support the EdgeX Foundry [Hanoi](https://www.edgexfoundry.org/software/releases/), and would not support upgrading/downgrading to other versions.k

## Implementation History

- [ ] 03/15/2021: Proposed idea in an issue or [community meeting](https://us02web.zoom.us/j/82828315928?pwd=SVVxek01T2Z0SVYraktCcDV4RmZlUT09)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting](https://us02web.zoom.us/j/82828315928?pwd=SVVxek01T2Z0SVYraktCcDV4RmZlUT09)
- [ ] MM/DD/YYYY: Open proposal PR
