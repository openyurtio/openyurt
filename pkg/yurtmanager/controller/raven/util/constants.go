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

package util

const (
	ConcurrentReconciles           = 1
	WorkingNamespace               = "kube-system"
	RavenGlobalConfig              = "raven-cfg"
	RavenAgentConfig               = "raven-agent-config"
	LabelCurrentGatewayEndpoints   = "raven.openyurt.io/endpoints-name"
	GatewayProxyInternalService    = "x-raven-proxy-internal-svc"
	GatewayProxyServiceNamePrefix  = "x-raven-proxy-svc"
	GatewayTunnelServiceNamePrefix = "x-raven-tunnel-svc"
	ExtraAllowedSourceCIDRs        = "raven.openyurt.io/extra-allowed-source-cidrs"

	RavenProxyNodesConfig      = "edge-tunnel-nodes"
	ProxyNodesKey              = "tunnel-nodes"
	ProxyServerSecurePortKey   = "proxy-internal-secure-addr"
	ProxyServerInsecurePortKey = "proxy-internal-insecure-addr"
	ProxyServerExposedPortKey  = "proxy-external-addr"
	VPNServerExposedPortKey    = "tunnel-bind-addr"
	RavenEnableProxy           = "enable-l7-proxy"
	RavenEnableTunnel          = "enable-l3-tunnel"
)
