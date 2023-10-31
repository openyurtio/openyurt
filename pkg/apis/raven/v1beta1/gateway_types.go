/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Event reason.
const (
	// EventActiveEndpointElected is the event indicating a new active endpoint is elected.
	EventActiveEndpointElected = "ActiveEndpointElected"
	// EventActiveEndpointLost is the event indicating the active endpoint is lost.
	EventActiveEndpointLost = "ActiveEndpointLost"
)

const (
	ExposeTypePublicIP     = "PublicIP"
	ExposeTypeLoadBalancer = "LoadBalancer"
)

const (
	Proxy  = "proxy"
	Tunnel = "tunnel"

	DefaultProxyServerSecurePort   = 10263
	DefaultProxyServerInsecurePort = 10264
	DefaultProxyServerExposedPort  = 10262
	DefaultTunnelServerExposedPort = 4500
)

// ProxyConfiguration is the configuration for raven l7 proxy
type ProxyConfiguration struct {
	// Replicas is the number of gateway active endpoints that enabled proxy
	Replicas int `json:"Replicas"`
	// ProxyHTTPPort is the proxy http port of the cross-domain request
	ProxyHTTPPort string `json:"proxyHTTPPort,omitempty"`
	// ProxyHTTPSPort is the proxy https port of the cross-domain request
	ProxyHTTPSPort string `json:"proxyHTTPSPort,omitempty"`
}

// TunnelConfiguration is the configuration for raven l3 tunnel
type TunnelConfiguration struct {
	// Replicas is the number of gateway active endpoints that enabled tunnel
	Replicas int `json:"Replicas"`
}

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// NodeSelector is a label query over nodes that managed by the gateway.
	// The nodes in the same gateway should share same layer 3 network.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// ProxyConfig determine the l7 proxy configuration
	ProxyConfig ProxyConfiguration `json:"proxyConfig,omitempty"`
	// TunnelConfig determine the l3 tunnel configuration
	TunnelConfig TunnelConfiguration `json:"tunnelConfig,omitempty"`
	// Endpoints are a list of available Endpoint.
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	// ExposeType determines how the Gateway is exposed.
	ExposeType string `json:"exposeType,omitempty"`
}

// Endpoint stores all essential data for establishing the VPN tunnel and Proxy
type Endpoint struct {
	// NodeName is the Node hosting this endpoint.
	NodeName string `json:"nodeName"`
	// Type is the service type of the node, proxy or tunnel
	Type string `json:"type"`
	// Port is the exposed port of the node
	Port int `json:"port,omitempty"`
	// UnderNAT indicates whether node is under NAT
	UnderNAT bool `json:"underNAT,omitempty"`
	// NATType is the NAT type of the node
	NATType string `json:"natType,omitempty"`
	// PublicIP is the exposed IP of the node
	PublicIP string `json:"publicIP,omitempty"`
	// PublicPort is the port used for NAT traversal
	PublicPort int `json:"publicPort,omitempty"`
	// Config is a map to record config for the raven agent of node
	Config map[string]string `json:"config,omitempty"`
}

// NodeInfo stores information of node managed by Gateway.
type NodeInfo struct {
	// NodeName is the Node host name.
	NodeName string `json:"nodeName"`
	// PrivateIP is the node private ip address
	PrivateIP string `json:"privateIP"`
	// Subnets is the pod ip range of the node
	Subnets []string `json:"subnets"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// Nodes contains all information of nodes managed by Gateway.
	Nodes []NodeInfo `json:"nodes,omitempty"`
	// ActiveEndpoints is the reference of the active endpoint.
	ActiveEndpoints []*Endpoint `json:"activeEndpoints,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=gateways,shortName=gw,categories=all
// +kubebuilder:storageversion

// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayList contains a list of Gateway
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}
