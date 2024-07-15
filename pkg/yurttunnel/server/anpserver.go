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
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"k8s.io/klog/v2"
	anpserver "sigs.k8s.io/apiserver-network-proxy/pkg/server"
	anpagent "sigs.k8s.io/apiserver-network-proxy/proto/agent"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	hw "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	wh "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/wraphandler"
)

// anpTunnelServer implements the TunnelServer interface using the
// apiserver-network-proxy package
type anpTunnelServer struct {
	egressSelectorEnabled    bool
	interceptorServerUDSFile string
	serverMasterAddr         string
	serverMasterInsecureAddr string
	serverAgentAddr          string
	serverCount              int
	tlsCfg                   *tls.Config
	proxyClientTlsCfg        *tls.Config
	wrappers                 hw.HandlerWrappers
	proxyStrategy            string
}

var _ TunnelServer = &anpTunnelServer{}

// Run runs the yurttunnel-server
func (ats *anpTunnelServer) Run() error {
	proxyServer := anpserver.NewProxyServer(uuid.New().String(),
		[]anpserver.ProxyStrategy{anpserver.ProxyStrategy(ats.proxyStrategy)},
		ats.serverCount,
		&anpserver.AgentTokenAuthenticationOptions{})
	// 1. start the proxier
	proxierErr := runProxier(
		&anpserver.Tunnel{Server: proxyServer},
		ats.egressSelectorEnabled,
		ats.interceptorServerUDSFile,
		ats.tlsCfg)
	if proxierErr != nil {
		return fmt.Errorf("could not run the proxier: %w", proxierErr)
	}

	wrappedHandler, err := wh.WrapHandler(
		NewRequestInterceptor(ats.interceptorServerUDSFile, ats.proxyClientTlsCfg),
		ats.wrappers,
	)
	if err != nil {
		return fmt.Errorf("could not wrap handler: %w", err)
	}

	// 2. start the master server
	masterServerErr := runMasterServer(
		wrappedHandler,
		ats.egressSelectorEnabled,
		ats.serverMasterAddr,
		ats.serverMasterInsecureAddr,
		ats.tlsCfg)
	if masterServerErr != nil {
		return fmt.Errorf("could not run master server: %w", masterServerErr)
	}

	// 3. start the agent server
	agentServerErr := runAgentServer(ats.tlsCfg, ats.serverAgentAddr, proxyServer)
	if agentServerErr != nil {
		return fmt.Errorf("could not run agent server: %w", agentServerErr)
	}

	return nil
}

// runProxier starts a proxy server that redirects requests received from
// apiserver to corresponding yurttunel-agent
func runProxier(handler http.Handler,
	egressSelectorEnabled bool,
	udsSockFile string,
	tlsConfig *tls.Config) error {
	klog.Info("start handling request from interceptor")
	if egressSelectorEnabled {
		// TODO will support egress selector for apiserver version > 1.18
		return errors.New("DOESN'T SUPPORT EGRESS SELECTOR YET")
	}
	// request will be sent from request interceptor on the same host,
	// so we use UDS protocol to avoid sending request through kernel
	// network stack.
	go func() {
		server := &http.Server{
			Handler:     handler,
			ReadTimeout: constants.YurttunnelANPProxierReadTimeoutSec * time.Second,
		}
		unixListener, err := net.Listen(constants.UnixListenerNetwork, udsSockFile)
		if err != nil {
			klog.Errorf("proxier could not serving request through uds: %s", err)
		}
		defer unixListener.Close()
		if err := server.Serve(unixListener); err != nil {
			klog.Errorf("proxier could not serving request through uds: %s", err)
		}
	}()

	return nil
}

// runMasterServer runs an https server to handle requests from apiserver
func runMasterServer(handler http.Handler,
	egressSelectorEnabled bool,
	masterServerAddr,
	masterServerInsecureAddr string,
	tlsCfg *tls.Config) error {
	if egressSelectorEnabled {
		return errors.New("DOESN'T SUPPORT EGRESS SELECTOR YET")
	}
	go func() {
		klog.Infof("start handling https request from master at %s", masterServerAddr)
		server := http.Server{
			Addr:         masterServerAddr,
			Handler:      handler,
			ReadTimeout:  constants.YurttunnelANPInterceptorReadTimeoutSec * time.Second,
			TLSConfig:    tlsCfg,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			klog.Errorf("could not serve https request from master: %v", err)
		}
	}()

	go func() {
		klog.Infof("start handling http request from master at %s", masterServerInsecureAddr)
		server := http.Server{
			Addr:         masterServerInsecureAddr,
			ReadTimeout:  constants.YurttunnelANPInterceptorReadTimeoutSec * time.Second,
			Handler:      handler,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		}
		if err := server.ListenAndServe(); err != nil {
			klog.Errorf("could not serve http request from master: %v", err)
		}
	}()

	return nil
}

// runAgentServer runs a grpc server that handles connections from the yurttunel-agent
// NOTE agent server is responsible for managing grpc connection yurttunel-server
// and yurttunnel-agent, and the proxy server is responsible for redirecting requests
// to corresponding yurttunel-agent
func runAgentServer(tlsCfg *tls.Config,
	agentServerAddr string,
	proxyServer *anpserver.ProxyServer) error {
	serverOption := grpc.Creds(credentials.NewTLS(tlsCfg))

	ka := keepalive.ServerParameters{
		// Ping the client if it is idle for `Time` seconds to ensure the
		// connection is still active
		Time: constants.YurttunnelANPGrpcKeepAliveTimeSec * time.Second,
		// Wait `Timeout` second for the ping ack before assuming the
		// connection is dead
		Timeout: constants.YurttunnelANPGrpcKeepAliveTimeoutSec * time.Second,
	}

	grpcServer := grpc.NewServer(serverOption,
		grpc.KeepaliveParams(ka))

	anpagent.RegisterAgentServiceServer(grpcServer, proxyServer)
	listener, err := net.Listen("tcp", agentServerAddr)
	klog.Info("start handling connection from agents")
	if err != nil {
		return fmt.Errorf("could not listen to agent on %s: %w", agentServerAddr, err)
	}
	go grpcServer.Serve(listener)
	return nil
}
