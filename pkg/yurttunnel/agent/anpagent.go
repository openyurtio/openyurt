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
	"crypto/tls"
	"net"
	"reflect"
	"time"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/server/serveraddr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	anpagent "sigs.k8s.io/apiserver-network-proxy/pkg/agent"
)

// anpTunnelAgent implements the TunnelAgent using the
// apiserver-network-proxy package
type anpTunnelAgent struct {
	tlsCfg           *tls.Config
	tunnelServerAddr string
	nodeName         string
	agentIdentifiers string
	client           kubernetes.Interface
	probInterval     time.Duration
}

var _ TunnelAgent = &anpTunnelAgent{}

// RunAgent runs the yurttunnel-agent which will try to connect yurttunnel-server
func (ata *anpTunnelAgent) Run(stopChan <-chan struct{}) {
	var (
		csStopChan = make(chan struct{})
		dialOption = grpc.WithTransportCredentials(credentials.NewTLS(ata.tlsCfg))
	)
	cc := &anpagent.ClientSetConfig{
		Address:                 ata.tunnelServerAddr,
		AgentID:                 ata.nodeName,
		AgentIdentifiers:        ata.agentIdentifiers,
		SyncInterval:            5 * time.Second,
		ProbeInterval:           5 * time.Second,
		DialOptions:             []grpc.DialOption{dialOption},
		ServiceAccountTokenPath: "",
	}

	cc.NewAgentClientSet(csStopChan).Serve()
	klog.Infof("start serving grpc request redirected from %s: %s",
		projectinfo.GetServerName(), ata.tunnelServerAddr)

	// probe tunnel server address and reconnect it if address changed.
	go func() {
		for {
			select {
			case <-stopChan:
				close(csStopChan)
				return
			case <-time.After(ata.probInterval):
				addr, err := serveraddr.GetTunnelServerAddr(ata.client)
				if err != nil {
					klog.Infof("get tunnel server addr err: %+v", err)
					continue
				}
				if !reflect.DeepEqual(addr, ata.tunnelServerAddr) {
					klog.Infof("tunnel server's ip has changed")
					host, _, err := net.SplitHostPort(addr)
					if err != nil {
						klog.Infof("split host port err: %+v", err)
					}
					// update related filed
					ata.tlsCfg.ServerName = host
					ata.tunnelServerAddr = addr
					dialOption = grpc.WithTransportCredentials(credentials.NewTLS(ata.tlsCfg))
					cc.Address = host
					// shutdown clientSet and recreate a new one to connect the new address
					csStopChan <- struct{}{}
					cc.NewAgentClientSet(csStopChan).Serve()
					klog.Infof("restart serving grpc request redirected from %s: %s",
						projectinfo.GetServerName(), ata.tunnelServerAddr)
				}
			}
		}
	}()
}
