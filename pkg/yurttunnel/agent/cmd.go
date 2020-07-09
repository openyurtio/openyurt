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

package agent

import (
	"errors"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	kubeutil "github.com/alibaba/openyurt/pkg/yurttunnel/kubernetes"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki/certmanager"
)

// NewYurttunnelAgentCommand creates a new yurttunnel-agent command
func NewYurttunnelAgentCommand(stopCh <-chan struct{}) *cobra.Command {
	o := &YurttunnelAgentOptions{}

	cmd := &cobra.Command{
		Short: "Launch yurttunnel-agent",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.validate(); err != nil {
				return err
			}
			if err := o.complete(); err != nil {
				return err
			}
			if err := o.run(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&o.nodeName, "node-name", o.nodeName,
		"The name of the edge node.")
	flags.StringVar(&o.serverAddr, "server-addr", o.serverAddr,
		"The address of yurttunnel-server")
	flags.StringVar(&o.kubeConfig, "kube-config", o.kubeConfig,
		"Path to the kubeconfig file")

	return cmd
}

// YurttunnelAgentOptions has the information that required by the
// yurttunel-agent
type YurttunnelAgentOptions struct {
	nodeName   string
	serverAddr string
	kubeConfig string
	clientset  kubernetes.Interface
}

// validate validates the YurttunnelServerOptions
func (o *YurttunnelAgentOptions) validate() error {
	if o.kubeConfig == "" {
		return errors.New("--kube-config is not set")
	}

	if o.nodeName == "" {
		return errors.New("--node-name is not set")
	}

	return nil
}

// complete completes all the required options
func (o *YurttunnelAgentOptions) complete() error {
	var err error
	klog.Infof("create clientset based on the kubeconfig(%s)", o.kubeConfig)
	o.clientset, err = kubeutil.CreateClientSetKubeConfig(o.kubeConfig)
	if err != nil {
		return err
	}
	return nil
}

// run starts the yurttunel-agent
func (o *YurttunnelAgentOptions) run(stopCh <-chan struct{}) error {
	var (
		serverAddr   string
		err          error
		agentCertMgr certificate.Manager
	)
	// 1. get the address of the yurttunnel-server
	serverAddr = o.serverAddr
	if o.serverAddr == "" {
		if serverAddr, err = GetServerAddr(o.kubeConfig); err != nil {
			return err
		}
	}
	klog.Infof("tunnel server address: %s", serverAddr)

	// 2. create a certificate manager
	agentCertMgr, err =
		certmanager.NewAgentCertManager(o.clientset, "yurttunnel-agent")
	if err != nil {
		return err
	}
	agentCertMgr.Start()

	// 3. generate a TLS configuration for securing the connection to server
	tlsCfg, err := pki.GenTLSConfigUseCertMgrAndCA(agentCertMgr,
		serverAddr, constants.YurttunnelAgentCAFile)
	if err != nil {
		return err
	}

	// 4. start the yurttunnel-agent
	if err := RunAgent(tlsCfg, serverAddr, o.nodeName, stopCh); err != nil {
		return err
	}
	<-stopCh
	return nil
}
