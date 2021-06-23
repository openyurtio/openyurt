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
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/profile"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
)

// Server is an interface for providing http service for yurthub
type Server interface {
	Run()
}

// yutHubServer includes hubServer and proxyServer,
// and hubServer handles requests by hub agent itself, like profiling, metrics, healthz
// and proxyServer does not handle requests locally and proxy requests to kube-apiserver
type yurtHubServer struct {
	CAFile string
	// CertFile the tls cert file for proxyhub's https server
	CertFile string
	// KeyFile the tls key file for proxyhub's https server
	KeyFile string

	hubServer         *http.Server
	proxyServer       *http.Server
	secureProxyServer *http.Server
	dummyProxyServer  *http.Server
}

// NewYurtHubServer creates a Server object
func NewYurtHubServer(cfg *config.YurtHubConfiguration,
	certificateMgr interfaces.YurtCertificateManager,
	proxyHandler http.Handler) (Server, error) {
	hubMux := mux.NewRouter()
	registerHandlers(hubMux, cfg, certificateMgr)
	hubServer := &http.Server{
		Addr:           cfg.YurtHubServerAddr,
		Handler:        hubMux,
		MaxHeaderBytes: 1 << 20,
	}

	proxyServer := &http.Server{
		Addr:    cfg.YurtHubProxyServerAddr,
		Handler: proxyHandler,
	}

	caFile, err := ioutil.ReadFile(cfg.CAFile)
	if err != nil {
		klog.Errorf("Read ca file err: %v", err)
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM([]byte(caFile))

	secureProxyServer := &http.Server{
		Addr:    cfg.YurtHubProxyServerSecureAddr,
		Handler: proxyHandler,
		TLSConfig: &tls.Config{
			ClientCAs:  certPool,
			ClientAuth: tls.VerifyClientCertIfGiven,
		},
		MaxHeaderBytes: 1 << 20,
	}

	var dummyProxyServer *http.Server
	if cfg.EnableDummyIf {
		if _, err := net.InterfaceByName(cfg.HubAgentDummyIfName); err != nil {
			return nil, err
		}

		dummyProxyServer = &http.Server{
			Addr:           cfg.YurtHubProxyServerDummyAddr,
			Handler:        proxyHandler,
			MaxHeaderBytes: 1 << 20,
		}
	}

	return &yurtHubServer{
		CAFile:            cfg.CAFile,
		CertFile:          cfg.CertFile,
		KeyFile:           cfg.KeyFile,
		hubServer:         hubServer,
		proxyServer:       proxyServer,
		secureProxyServer: secureProxyServer,
		dummyProxyServer:  dummyProxyServer,
	}, nil
}

// Run will start hub server and proxy server
func (s *yurtHubServer) Run() {
	go func() {
		err := s.hubServer.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()

	if s.dummyProxyServer != nil {
		go func() {
			err := s.dummyProxyServer.ListenAndServe()
			if err != nil {
				panic(err)
			}
		}()
	}

	go func() {
		err := s.secureProxyServer.ListenAndServeTLS(s.CertFile, s.KeyFile)
		if err != nil {
			panic(err)
		}
	}()

	err := s.proxyServer.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

// registerHandler registers handlers for yurtHubServer, and yurtHubServer can handle requests like profiling, healthz, update token.
func registerHandlers(c *mux.Router, cfg *config.YurtHubConfiguration, certificateMgr interfaces.YurtCertificateManager) {
	// register handlers for update join token
	c.Handle("/v1/token", updateTokenHandler(certificateMgr)).Methods("POST", "PUT")

	// register handler for health check
	c.HandleFunc("/v1/healthz", healthz).Methods("GET")

	// register handler for profile
	if cfg.EnableProfiling {
		profile.Install(c)
	}

	// register handler for metrics
	c.Handle("/metrics", promhttp.Handler())
}

// healthz returns ok for healthz request
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}
