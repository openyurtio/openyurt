/*
Copyright 2021 The OpenYurt Authors.

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

package config

import (
	"crypto/x509"
	"fmt"
	"net"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/iptables"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

// Config is the main context object for yurttunel-server
type Config struct {
	EgressSelectorEnabled       bool
	EnableIptables              bool
	EnableDNSController         bool
	IptablesSyncPeriod          int
	IPFamily                    iptables.Protocol
	DNSSyncPeriod               int
	CertDNSNames                []string
	CertIPs                     []net.IP
	CertDir                     string
	ListenAddrForAgent          string
	ListenAddrForMaster         string
	ListenInsecureAddrForMaster string
	ListenMetaAddr              string
	RootCert                    *x509.CertPool
	Client                      kubernetes.Interface
	SharedInformerFactory       informers.SharedInformerFactory
	ServerCount                 int
	ProxyStrategy               string
	InterceptorServerUDSFile    string
}

type completedConfig struct {
	*Config
}

// CompletedConfig same as Config, just to swap private object.
type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *Config) Complete() *CompletedConfig {
	cc := completedConfig{c}

	if cc.InterceptorServerUDSFile == "" {
		cc.InterceptorServerUDSFile = "/tmp/interceptor-proxier.sock"
	}
	if cc.CertDir == "" {
		cc.CertDir = fmt.Sprintf(constants.YurttunnelServerCertDir, projectinfo.GetServerName())
	}
	return &CompletedConfig{&cc}
}

func (c *Config) IsIPv6() bool {
	return c.IPFamily == iptables.ProtocolIpv6
}
