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
	"flag"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	kubeutil "github.com/alibaba/openyurt/pkg/yurttunnel/kubernetes"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki/certmanager"
	"github.com/alibaba/openyurt/pkg/yurttunnel/projectinfo"
)

const defaultKubeconfig = "/etc/kubernetes/kubelet.conf"

// NewYurttunnelAgentCommand creates a new yurttunnel-agent command
func NewYurttunnelAgentCommand(stopCh <-chan struct{}) *cobra.Command {
	o := &YurttunnelAgentOptions{}

	cmd := &cobra.Command{
		Short: fmt.Sprintf("Launch %s", projectinfo.GetAgentName()),
		RunE: func(c *cobra.Command, args []string) error {
			if o.version {
				fmt.Println(projectinfo.ShortAgentVersion())
				return nil
			}
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
	flags.BoolVar(&o.version, "version", o.version,
		"print the version information.")
	flags.StringVar(&o.nodeName, "node-name", o.nodeName,
		"The name of the edge node.")
	flags.StringVar(&o.tunnelServerAddr, "tunnelserver-addr", o.tunnelServerAddr,
		fmt.Sprintf("The address of %s", projectinfo.GetServerName()))
	flags.StringVar(&o.apiserverAddr, "apiserver-addr", o.tunnelServerAddr,
		"A reachable address of the apiserver.")
	flags.StringVar(&o.kubeConfig, "kube-config", o.kubeConfig,
		"Path to the kubeconfig file.")

	// add klog flags as the global flagsets
	klog.InitFlags(nil)
	flags.AddGoFlagSet(flag.CommandLine)
	return cmd
}

// YurttunnelAgentOptions has the information that required by the
// yurttunel-agent
type YurttunnelAgentOptions struct {
	nodeName         string
	tunnelServerAddr string
	apiserverAddr    string
	kubeConfig       string
	version          bool
	clientset        kubernetes.Interface
}

// validate validates the YurttunnelServerOptions
func (o *YurttunnelAgentOptions) validate() error {
	if o.nodeName == "" {
		return errors.New("--node-name is not set")
	}

	return nil
}

// complete completes all the required options
func (o *YurttunnelAgentOptions) complete() error {
	var err error

	if o.kubeConfig == "" && o.apiserverAddr == "" {
		o.kubeConfig = defaultKubeconfig
		klog.Infof("neither --kube-config nor --apiserver-addr is set, will use %s as the kubeconfig", o.kubeConfig)
	}

	if o.kubeConfig != "" {
		klog.Infof("create the clientset based on the kubeconfig(%s).", o.kubeConfig)
		o.clientset, err = kubeutil.CreateClientSetKubeConfig(o.kubeConfig)
		return err
	}

	klog.Infof("create the clientset based on the apiserver address(%s).", o.apiserverAddr)
	o.clientset, err = kubeutil.CreateClientSetApiserverAddr(o.apiserverAddr)
	return err
}

// run starts the yurttunel-agent
func (o *YurttunnelAgentOptions) run(stopCh <-chan struct{}) error {
	var (
		tunnelServerAddr string
		err              error
		agentCertMgr     certificate.Manager
	)

	// 1. get the address of the yurttunnel-server
	tunnelServerAddr = o.tunnelServerAddr
	if o.tunnelServerAddr == "" {
		if tunnelServerAddr, err = GetTunnelServerAddr(o.clientset); err != nil {
			return err
		}
	}
	klog.Infof("%s address: %s", projectinfo.GetServerName(), tunnelServerAddr)

	// 2. create a certificate manager
	agentCertMgr, err =
		certmanager.NewYurttunnelAgentCertManager(o.clientset)
	if err != nil {
		return err
	}
	agentCertMgr.Start()

	// 3. generate a TLS configuration for securing the connection to server
	tlsCfg, err := pki.GenTLSConfigUseCertMgrAndCA(agentCertMgr,
		tunnelServerAddr, constants.YurttunnelCAFile)
	if err != nil {
		return err
	}

	// 4. start the yurttunnel-agent
	RunAgent(tlsCfg, tunnelServerAddr, o.nodeName, stopCh)

	<-stopCh
	return nil
}
