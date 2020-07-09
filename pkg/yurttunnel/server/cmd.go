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
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	"github.com/alibaba/openyurt/pkg/yurttunnel/iptables"
	"github.com/alibaba/openyurt/pkg/yurttunnel/kubernetes"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki/certmanager"
)

// NewYurttunnelServerCommand creates a new yurttunnel-server command
func NewYurttunnelServerCommand(stopCh <-chan struct{}) *cobra.Command {
	o := NewYurttunnelServerOptions()

	cmd := &cobra.Command{
		Use:   "yurttunnel-server",
		Short: "yurttunnel-server sends requests to yurttunnel-agents",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.validate(); err != nil {
				return err
			}
			o.complete()
			if err := o.run(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&o.kubeConfig, "kube-config", o.kubeConfig,
		"path to the kubeconfig file.")
	flags.StringVar(&o.bindAddr, "bind-address", o.bindAddr,
		"the ip address on which the server will listen.")
	flags.StringVar(&o.caFile, "ca-file", o.caFile,
		"CA file for connecting to edge tunnel agent and kubelet.")
	flags.BoolVar(&o.enableIptables, "enable-iptables", o.enableIptables,
		"if allow iptable manager to set the dnat rule.")
	flags.IntVar(&o.iptablesSyncPeriod, "iptables-sync-period", o.iptablesSyncPeriod,
		"the synchronization period of the iptable manager.")
	return cmd
}

// YurttunnelServerOptions has the information that required by the
// yurttunel-server
type YurttunnelServerOptions struct {
	kubeConfig               string
	bindAddr                 string
	caFile                   string
	enableIptables           bool
	iptablesSyncPeriod       int
	serverAgentPort          int
	serverMasterPort         int
	interceptorServerUDSFile string
	serverAgentAddr          string
	serverMasterAddr         string
}

// NewYurttunnelServerOptions creates a new YurtNewYurttunnelServerOptions
func NewYurttunnelServerOptions() *YurttunnelServerOptions {
	o := &YurttunnelServerOptions{
		caFile:                   "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		bindAddr:                 "0.0.0.0",
		enableIptables:           true,
		iptablesSyncPeriod:       60,
		serverAgentPort:          constants.YurttunnelServerAgentPort,
		serverMasterPort:         constants.YurttunnelServerMasterPort,
		interceptorServerUDSFile: "/tmp/interceptor-proxier.sock",
	}
	return o
}

// validate validates the YurttunnelServerOptions
func (o *YurttunnelServerOptions) validate() error {
	if len(o.bindAddr) == 0 {
		return fmt.Errorf("tunnel server's bind address can't be empty")
	}
	return nil
}

// complete completes all the required options
func (o *YurttunnelServerOptions) complete() {
	o.serverAgentAddr = fmt.Sprintf("%s:%d", o.bindAddr, o.serverAgentPort)
	o.serverMasterAddr = fmt.Sprintf("%s:%d", o.bindAddr, o.serverMasterPort)
	klog.Infof("server will accept agent requests at: %s, "+
		"server will accept master requests at: %s",
		o.serverAgentAddr, o.serverMasterAddr)
}

// run starts the yurttunel-server
func (o *YurttunnelServerOptions) run(stopCh <-chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 1. create the master's clientset
	clientSet, csErr := kubernetes.CreateClientSet(o.kubeConfig)
	if csErr != nil {
		return fmt.Errorf("fail to create the clientset: %s", csErr)
	}

	// 2. start the IPNodename manager
	nodeIP2Name := make(map[string]string)
	nodeIndexer, ipNodeMgrErr :=
		StartIPNodeNameManager(clientSet, nodeIP2Name, stopCh)
	if ipNodeMgrErr != nil {
		return fmt.Errorf("fail to start ip node manager: %s", ipNodeMgrErr)
	}

	// 3. start the IP table manager
	if o.enableIptables {
		iptablesMgr := iptables.NewIptablesManager(clientSet,
			corelisters.NewNodeLister(nodeIndexer),
			o.bindAddr,
			o.iptablesSyncPeriod,
			stopCh)
		if iptablesMgr == nil {
			return fmt.Errorf("fail to create a new IptableManager")
		}
		iptablesMgr.Run()
	}

	// 4. start the certificate manager for the tunnel server
	serverCertMgr, err := certmanager.NewServerCertManager(
		clientSet, "", "proxy-server", stopCh)
	if err != nil {
		return err
	}
	serverCertMgr.Start()

	// 5. get the latest certificate
	_ = wait.PollUntil(5*time.Second, func() (bool, error) {
		if serverCertMgr.Current() != nil {
			return true, nil
		}
		klog.Infof("waiting for the master to sign the server certificate")
		return false, nil
	}, stopCh)

	// 6. generate the TLS configuration based on the latest certificate
	rootCertPool, err := pki.GenRootCertPool(o.kubeConfig, o.caFile)
	if err != nil {
		return fmt.Errorf("fail to generate the rootCertPool: %s", err)
	}
	tlsCfg, err :=
		pki.GenTLSConfigUseCertMgrAndCertPool(serverCertMgr, rootCertPool)
	if err != nil {
		return err
	}

	// 7. start the server
	if err := RunServer(ctx, o.interceptorServerUDSFile, o.serverMasterAddr,
		o.serverAgentAddr, tlsCfg, nodeIP2Name); err != nil {
		return err
	}

	<-stopCh
	return nil
}
