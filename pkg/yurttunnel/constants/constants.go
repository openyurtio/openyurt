/*
Copyright 2020 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package constants

const (
	YurttunnelServerAgentPort          = 10262
	YurttunnelServerMasterPort         = 10263
	YurttunnelServerMasterInsecurePort = 10264
	YurttunnelServerServiceNs          = "kube-system"
	YurttunnelServerServiceName        = "x-tunnel-server-svc"
	YurttunnelServerAgentPortName      = "tcp"
	YurttunnelServerExternalAddrKey    = "x-tunnel-server-external-addr"
	YurttunnelEndpointsNs              = "kube-system"
	YurttunnelEndpointsName            = "x-tunnel-server-svc"

	// yurttunnel PKI related constants
	YurttunnelCSROrg                 = "openyurt:yurttunnel"
	YurttunnelAgentCSRCN             = "yurttunnel-agent"
	YurttunneServerCSROrg            = "system:masters"
	YurttunneServerCSRCN             = "kube-apiserver-kubelet-client"
	YurttunnelCAFile                 = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	YurttunnelTokenFile              = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	YurttunnelServerCertDir          = "/var/lib/yurt-tunnel-server/pki"
	YurttunnelAgentCertDir           = "/var/lib/yurt-tunnel-agent/pki"
	YurttunnelCSRApproverThreadiness = 2

	// name of the environment variables used in pod
	YurttunnelAgentPodIPEnv = "POD_IP"

	// The timeout seconds of reading a complete request from the apiserver
	YurttunnelANPInterceptorReadTimeoutSec = 10
	// The period between two keep-alive probes
	YurttunnelANPInterceptorKeepAlivePeriodSec = 10
	// The timeout seconds for the interceptor to proceed a complete read from the proxier
	YurttunnelANPProxierReadTimeoutSec = 10
	// probe the client every 10 seconds to ensure the connection is still active
	YurttunnelANPGrpcKeepAliveTimeSec = 10
	// wait 5 seconds for the probe ack before cutting the connection
	YurttunnelANPGrpcKeepAliveTimeoutSec = 5
)
