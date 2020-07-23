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
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/klog"

	anpserver "sigs.k8s.io/apiserver-network-proxy/pkg/server"
	anpagent "sigs.k8s.io/apiserver-network-proxy/proto/agent"
)

// RunServer runs the yurttunnel-server
func RunServer(ctx context.Context,
	egressSelectorEnabled bool,
	interceptorServerUDSFile,
	serverMasterAddr,
	serverMasterInsecureAddr,
	serverAgentAddr string,
	tlsCfg *tls.Config) error {
	proxyServer := anpserver.NewProxyServer(uuid.New().String(), 1,
		&anpserver.AgentTokenAuthenticationOptions{})

	// 1. start the proxier
	proxierErr := runProxier(&anpserver.Tunnel{Server: proxyServer},
		egressSelectorEnabled,
		interceptorServerUDSFile,
		tlsCfg)
	if proxierErr != nil {
		return fmt.Errorf("fail to run the proxier: %s", proxierErr)
	}

	// 2. start the master server
	masterServerErr := runMasterServer(
		&RequestInterceptor{
			UDSSockFile: interceptorServerUDSFile,
			TLSConfig:   tlsCfg,
		},
		egressSelectorEnabled,
		serverMasterAddr,
		serverMasterInsecureAddr,
		tlsCfg)
	if masterServerErr != nil {
		return fmt.Errorf("fail to run master server: %s", masterServerErr)
	}

	// 3. start the agent server
	agentServerErr := runAgentServer(tlsCfg, serverAgentAddr, proxyServer)
	if agentServerErr != nil {
		return fmt.Errorf("fail to run agent server: %s", agentServerErr)
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
		return errors.New("DOESN'T SUPPROT EGRESS SELECTOR YET")
	}
	// request will be sent from request interceptor on the same host,
	// so we use UDS protocol to avoide sending request through kernel
	// network stack.
	go func() {
		server := &http.Server{
			Handler: handler,
		}
		unixListener, err := net.Listen("unix", udsSockFile)
		if err != nil {
			klog.Errorf("proxier fail to serving request through uds: %s", err)
		}
		defer unixListener.Close()
		if err := server.Serve(unixListener); err != nil {
			klog.Errorf("proxier fail to serving request through uds: %s", err)
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
			TLSConfig:    tlsCfg,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			klog.Errorf("failed to serve https request from master: %v", err)
		}
	}()

	go func() {
		klog.Infof("start handling http request from master at %s", masterServerInsecureAddr)
		server := http.Server{
			Addr:         masterServerInsecureAddr,
			Handler:      handler,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		}
		if err := server.ListenAndServe(); err != nil {
			klog.Errorf("failed to serve http request from master: %v", err)
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
	grpcServer := grpc.NewServer(serverOption)
	anpagent.RegisterAgentServiceServer(grpcServer, proxyServer)
	listener, err := net.Listen("tcp", agentServerAddr)
	klog.Info("start handling connection from agents")
	if err != nil {
		return fmt.Errorf("fail to listen to agent on %s: %s", agentServerAddr, err)
	}
	go grpcServer.Serve(listener)
	return nil
}
