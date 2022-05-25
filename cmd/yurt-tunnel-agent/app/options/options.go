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

package options

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"
	"sigs.k8s.io/apiserver-network-proxy/pkg/agent"

	"github.com/openyurtio/openyurt/cmd/yurt-tunnel-agent/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	utilip "github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurttunnel/kubernetes"
)

const defaultKubeconfig = "/etc/kubernetes/kubelet.conf"

// AgentOptions has the information that required by the yurttunel-agent
type AgentOptions struct {
	NodeName         string
	NodeIP           string
	TunnelServerAddr string
	ApiserverAddr    string
	KubeConfig       string
	Version          bool
	AgentIdentifiers string
	MetaHost         string
	MetaPort         string
	CertDir          string
}

// NewAgentOptions creates a new AgentOptions with a default config.
func NewAgentOptions() *AgentOptions {
	o := &AgentOptions{
		MetaPort: constants.YurttunnelAgentMetaPort,
	}

	return o
}

// validate validates the AgentOptions
func (o *AgentOptions) Validate() error {
	if o.NodeName == "" {
		o.NodeName = os.Getenv("NODE_NAME")
		if o.NodeName == "" {
			return errors.New("either --node-name or $NODE_NAME has to be set")
		}
	}

	if o.NodeIP == "" {
		o.NodeIP = os.Getenv("NODE_IP")
		if o.NodeIP == "" {
			return errors.New("either --node-ip or $NODE_IP has to be set")
		}
	}

	if o.MetaHost == "" {
		o.MetaHost = utilip.MustGetLoopbackIP(utilnet.IsIPv6String(o.NodeIP))
	}

	if !agentIdentifiersAreValid(o.AgentIdentifiers) {
		return errors.New("--agent-identifiers are invalid, format should be host={node-name}")
	}

	return nil
}

// AddFlags returns flags for a specific yurttunnel-agent by section name
func (o *AgentOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
	fs.StringVar(&o.NodeName, "node-name", o.NodeName, "The name of the edge node.")
	fs.StringVar(&o.NodeIP, "node-ip", o.NodeIP, "The host IP of the edge node.")
	fs.StringVar(&o.TunnelServerAddr, "tunnelserver-addr", o.TunnelServerAddr, fmt.Sprintf("The address of %s", projectinfo.GetServerName()))
	fs.StringVar(&o.ApiserverAddr, "apiserver-addr", o.ApiserverAddr, "A reachable address of the apiserver.")
	fs.StringVar(&o.KubeConfig, "kube-config", o.KubeConfig, "Path to the kubeconfig file.")
	fs.StringVar(&o.AgentIdentifiers, "agent-identifiers", o.AgentIdentifiers, "The identifiers of the agent, which will be used by the server when choosing agent.")
	fs.StringVar(&o.MetaHost, "meta-host", o.MetaHost, "The ip address on which listen for --meta-port port.")
	fs.StringVar(&o.MetaPort, "meta-port", o.MetaPort, "The port on which to serve HTTP requests like profling, metrics")
	fs.StringVar(&o.CertDir, "cert-dir", o.CertDir, "The directory of certificate stored at.")
}

// agentIdentifiersIsValid verify agent identifiers are valid or not.
// and agentIdentifiers can be empty because default value will be set in complete() func.
func agentIdentifiersAreValid(agentIdentifiers string) bool {
	if len(agentIdentifiers) == 0 {
		return true
	}

	entries := strings.Split(agentIdentifiers, ",")
	for i := range entries {
		parts := strings.Split(entries[i], "=")
		if len(parts) != 2 {
			return false
		}

		switch agent.IdentifierType(parts[0]) {
		case agent.Host, agent.CIDR, agent.IPv4, agent.IPv6, agent.UID:
			// valid agent identifier
		default:
			return false
		}
	}

	return true
}

// Config return a yurttunnel agent config objective
func (o *AgentOptions) Config() (*config.Config, error) {
	var err error
	c := &config.Config{
		NodeName:         o.NodeName,
		NodeIP:           o.NodeIP,
		TunnelServerAddr: o.TunnelServerAddr,
		AgentIdentifiers: o.AgentIdentifiers,
		AgentMetaAddr:    net.JoinHostPort(o.MetaHost, o.MetaPort),
		CertDir:          o.CertDir,
	}

	if len(c.AgentIdentifiers) == 0 {
		ipFamily := "ipv4"
		if utilnet.IsIPv6String(o.NodeIP) {
			ipFamily = "ipv6"
		}
		c.AgentIdentifiers = fmt.Sprintf("%s=%s&host=%s", ipFamily, o.NodeIP, o.NodeName)
	}
	klog.Infof("%s is set for agent identifies", c.AgentIdentifiers)

	kubeConfig := o.KubeConfig
	if o.KubeConfig == "" && o.ApiserverAddr == "" {
		kubeConfig = defaultKubeconfig
		klog.Infof("neither --kube-config nor --apiserver-addr is set, will use %s as the kubeconfig", kubeConfig)
	}

	if kubeConfig != "" {
		klog.Infof("create the clientset based on the kubeconfig(%s).", kubeConfig)
		c.Client, err = kubeutil.CreateClientSetKubeConfig(kubeConfig)
		return c, err
	}

	klog.Infof("create the clientset based on the apiserver address(%s).", o.ApiserverAddr)
	c.Client, err = kubeutil.CreateClientSetApiserverAddr(o.ApiserverAddr)
	return c, err
}
