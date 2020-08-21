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

package server

import (
	"crypto/tls"
)

// TunnelServermanages tunnels between itself and agents, receives requests
// from apiserver, and forwards requests to corresponding agents
type TunnelServer interface {
	Run() error
}

// NewTunnelServer returns a new TunnelServer
func NewTunnelServer(
	egressSelectorEnabled bool,
	interceptorServerUDSFile,
	serverMasterAddr,
	serverMasterInsecureAddr,
	serverAgentAddr string,
	serverCount int,
	tlsCfg *tls.Config) TunnelServer {
	ats := anpTunnelServer{
		egressSelectorEnabled:    egressSelectorEnabled,
		interceptorServerUDSFile: interceptorServerUDSFile,
		serverMasterAddr:         serverMasterAddr,
		serverMasterInsecureAddr: serverMasterInsecureAddr,
		serverAgentAddr:          serverAgentAddr,
		serverCount:              serverCount,
		tlsCfg:                   tlsCfg,
	}
	return &ats
}
