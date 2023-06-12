---
title: support raven-l7-proxy 
authors:
  - "@River-sh"
reviewers:
  - ""
  - ""

creation-date: 2023-06-12
last-updated: 2023-x-x
---

# Support raven l7 proxy for cross-domain service proxy

## Table of Contents

- [support raven l7 proxy](#support raven l7 proxy)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [Gateway API](#gateway-api)
    - [Config Controller/Webhook](#config-controllerwebhook)
    - [Service Controller](#service-controller)
    - [DNS Controller](#dns-controller)
    - [Cert Manager](#certmanager)
    - [Raven L7 Proxy](#raven-l7-proxy-logic)
  - [Further optimization](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Implementation History](#implementation-history)

## Summary

In the edge scenario, the cloud and the edge are usually on different network domain, so the host and container networks 
across the network domain are not interconnected. raven-l3 can solve cross-domain communication in the absence of IP 
address conflict, raven still needs to be enhanced to support cross-domain access to host services in the case of IP conflict
This proposal aims to enhance raven network capabilities and replace yurt-tunnel.

## Motivation


Enhance raven network capability to support cross-domain access to host services in the case of IP conflict

### Goals

Enhanced Raven networking to replace Yurt-Tunnel, we want to achieve the following goals:
- Implement 7 layer proxy in the Raven project
- Maintain uniform architecture and design with Raven L3
- Do not rely too heavily on the control plane

### Non-Goals/Future Work

- Re-implement the reverse proxy tunnel packages into Raven repo instead of using ANP

## Proposal

- Raven L3 Architecture:

1. RavenAgent is deployed on each node of the cluster in DaemonSet mode, and the container is deployed in host network mode
2. The container network within the network domain can communicate with each other, and cross-domain requests are forwarded to the Gateway node for forwarding
3. RavenAgent on each node determines the routing configuration based on the CR configuration information of Gateway
4. Raven Agent on the Gateway node establishes a VPN channel with the Gateway exposed on the public network.

<img src = "../img/networking/img-4.png" width="800">

- Raven L7 Architecture:

1. RavenAgent is deployed on each node of the cluster in DaemonSet mode, and the container is deployed in host network mode
2. The container network within the network domain can communicate with each other, and cross-domain requests are forwarded 
to the Gateway node for forwarding
3. RavenAgent will start service according to its identity: ProxyClient will enable if it is not exposed to the public, 
ProxyServer will be started, if it is exposed to the public network
4. ProxyClient obtains the address of the exposed ProxyServer through the Gateway CR and actively establishes link
5. Requests across network domains are forwarded by ProxyServer proxy to ProxyClient in other network domains.


<img src = "../img/networking/img-5.png" width="800">


### Gateway API
1. The Endpoint property adds the type (L7-Proxy or L3-Tunnel)
2. The Endpoint property adds the port
3. Spec add attributes: EnableL7Proxy and EnableL3Tunnel to determine enable which service
4. Spec add attribute replicas to determine how many replicas 
5. ExposeType support three mode: NodePort、LoadBalancer、Normal
6. Support elect multi active endpoints

```go
// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	
	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// NodeSelector is a label query over nodes that managed by the gateway. 
	// The nodes in the same gateway should share same layer 3 network. 
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"` 
	// Endpoints is a list of available Endpoint. 
	Endpoints []Endpoint `json:"endpoints"` 
	// EnableServerProxy determine whether to enable the server proxy 
	EnableL7Proxy bool `json:"EnableL7Proxy",omitempty` 
	// EnableProxyClient determine whether to enable the network proxy 
	EnableL3Tunnel bool `json:"EnableL3Tunnel,omitempty"`
	// ExposeType determines how the gateway is exposed.
	ExposeType ExposeType `json:"exposeType,omitempty"` 
	// Replicas determine how many gateway is elected 
	Replicas int `json:"Replicas,omitempty"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// Nodes contains all information of nodes managed by Gateway.
	Nodes []NodeInfo `json:"nodes,omitempty"`
	// ActiveEndpoint is the reference of the active endpoint.
	ActiveEndpoints []*Endpoint `json:"activeEndpoints,omitempty"`    
}

// Endpoint stores all essential data for establishing the VPN tunnel.
type Endpoint struct {
	// NodeName is the Node hosting this endpoint. 
	NodeName  string            `json:"nodeName"`
	Type      string            `json:"type"`
	UnderNAT  bool              `json:"underNAT,omitempty"`
	PublicIP  string            `json:"publicIP,omitempty"`
	Port      string            `json:"port,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
   
}

// NodeInfo stores information of node managed by Gateway.
type NodeInfo struct {
	NodeName  string   `json:"nodeName"`
	PrivateIP string   `json:"privateIP"`
	Subnets   []string `json:"subnets"`
}
```

### Config Controller/Webhook

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: raven-cfg
  namespace: kube-system
  labels:
    app: raven
data:
  enable-l7-proxy: true
  enable-l3-tunnel: false
  gateway-replicas: 1
  l7-proxy-port: |
  https=10250
  http=10255
  exposed-gateway: | 
  gw-cloud=LoadBalancer
  gateway-nodes: |
  gw-cloud=cloud-node-1
  gw-edge=edge-node-1
```

1. ConfigController need to watch a configmap named raven-cfg
2. Configure CR gateway ``EnableL3Tunnel`` based ``enable-l3-tunnel``
3. Configure CR gateway ``EnableL7Proxy`` based ``enable-l7-proxy``
4. Configure CR gateway ``Replicas`` based ``gateway-replicas``
5. Configure selected CR gateway ``ExposedType`` base ``exposed-gateway``
6. Configure selected CR gateway ``Endpoints`` based ``gateway-nodes``
7. Configure proxy port for raven based ``l7-proxy-port``

### Gateway Controller/Webhook

1. GatewayController need watch node and gateway
2. Raven L3 and Raven about the elected separately in accordance with the requirements of the gateway node, 
according to the label of dividing network domain: ```raven.openyurt.io/gateway = ${Name}```
3. Raven L3 listen port 4500, raven l7 listen port 10262 and 10263
4. Webhook is aimed to check parameter limits

### Service Controller
1. ServiceController need watch gateway and a configmap named raven-cfg
2. Create/Update/Delete raven-svc and raven-internal-svc based on the Gateway
3. Each service is labeled ```raven.openyurt.io/gateway=${GatewayName}```
4. Select the Raven Agent on the gateway node as endpoints for these services
5. Manage the proxy port according to configmap raven-cfg

### DNS Controller
1. DNSController need watch node, gateway, a service raven-internal-svc, configmap named raven-cfg
2. Create/Update/Delete a configmap to record dns for coredns
3. Each dns configmap is labeled ```raven.openyurt.io/gateway=${GatewayName}```
4. DNS records follow the following rules: All nodes in the network domain are configured with node IP,
and all nodes outside the network domain are configured with raven-internal-svc IP

### CertManager
1. A certificate is generated for each ProxyServer and approved by the csrapprover
2. A certificate is generated for each ProxyClient and approved by the csrapprover
3. A certificate is generated for each ProxyServer to establishes a tls link with kubelet and approved by csrapprover

### Proxy principle

- ProxyClient obtains the public IP address of the ProxyServer and initiates a gRPC link request through the watch gateway
- ProxyServer receive requests and registers these ProxyClients as backend
- The Http request that across the network domain will be hijacked by the Interceptor and the header will be modified
- The Interceptor establishes a socket link with the Proxy and forwards the request to the Proxy
- The Proxy sends a dial request to the destination services in other network domains to establish tunnels
- In this case, the cross-domain tunnel is smooth, start data transmission

<img src = "../img/networking/img-6.png" width="800">

### Raven L7 proxy logic
<img src = "../img/networking/img-5.png" width="800">

- Gateway,service,dns,config controller and webhook are unified management by yurt-manager
- Raven-cfg and raven-agent are created when raven agent is deployed
- Each node should be labeled ```raven.openyurt.io/gateway=${GatewayName}``` to divide network domains and create a gateway CR f
or each network domain. We expect each node pool to be a network domain, but currently there is no mandatory limit for node pools
- Config controller watch raven-cfg to create/update all gateway CR
- Gateway controller elect gateway node and update gateway status
- Raven agent watch gateway and identify themselves to enable service
  1. The Raven Agent on the Gateway node exposed to the public network starts the ProxyServer
  2. The Raven Agent on the Gateway node not exposed to the public network starts the ProxyClient
  3. The Raven Agent on the node that is not belong to any gateway or has an exclusive gateway starts the ProxyClient
- Cert manager request three certificates
- Dns and service controller configure service and dns configmap
- Communication within the network domain is through the host network, and communication across the network domain is forward by raven proxy

## Further optimization
- Re-implement a reverse tunnel scheme as a Raven project package to replace ANP
- Optimize network links and remove Interceptor

### User Stories

#### Story 1
As an end user, I want to make some DevOps from Cloud to Edge, such as kubectl logs/exec.
#### Story 2
As an end user, I want to get the edge nodes metrics status through Prometheus/Metrics server from Cloud.
#### Story 3
As an end user, I want to access another business pod data from one NodePool to another NodePool.


## Implementation History

- [ ] 06/12/2023: Draft proposal created
- [ ] 06/14/2022: Present proposal at the community meeting

