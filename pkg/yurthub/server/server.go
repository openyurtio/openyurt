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
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/profile"
	"github.com/openyurtio/openyurt/pkg/util/certmanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	ota "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate"
)

// Server is an interface for providing http service for yurthub
type Server interface {
	Run()
}

// yutHubServer includes hubServer and proxyServer,
// and hubServer handles requests by hub agent itself, like profiling, metrics, healthz
// and proxyServer does not handle requests locally and proxy requests to kube-apiserver
type yurtHubServer struct {
	hubServer              *http.Server
	proxyServer            *http.Server
	secureProxyServer      *http.Server
	dummyProxyServer       *http.Server
	dummySecureProxyServer *http.Server
}

// NewYurtHubServer creates a Server object
func NewYurtHubServer(cfg *config.YurtHubConfiguration,
	certificateMgr certificate.YurtCertificateManager,
	proxyHandler http.Handler,
	rest *rest.RestConfigManager) (Server, error) {
	hubMux := mux.NewRouter()
	registerHandlers(hubMux, cfg, certificateMgr, rest)
	hubServer := &http.Server{
		Addr:           cfg.YurtHubServerAddr,
		Handler:        hubMux,
		MaxHeaderBytes: 1 << 20,
	}

	proxyHandler = wrapNonResourceHandler(proxyHandler, cfg, rest)
	proxyServer := &http.Server{
		Addr:    cfg.YurtHubProxyServerAddr,
		Handler: proxyHandler,
	}

	secureProxyServer := &http.Server{
		Addr:           cfg.YurtHubProxyServerSecureAddr,
		Handler:        proxyHandler,
		TLSConfig:      cfg.TLSConfig,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		MaxHeaderBytes: 1 << 20,
	}

	var dummyProxyServer, secureDummyProxyServer *http.Server
	if cfg.EnableDummyIf {
		if _, err := net.InterfaceByName(cfg.HubAgentDummyIfName); err != nil {
			return nil, err
		}

		dummyProxyServer = &http.Server{
			Addr:           cfg.YurtHubProxyServerDummyAddr,
			Handler:        proxyHandler,
			MaxHeaderBytes: 1 << 20,
		}

		secureDummyProxyServer = &http.Server{
			Addr:           cfg.YurtHubProxyServerSecureDummyAddr,
			Handler:        proxyHandler,
			TLSConfig:      cfg.TLSConfig,
			TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
			MaxHeaderBytes: 1 << 20,
		}
	}

	return &yurtHubServer{
		hubServer:              hubServer,
		proxyServer:            proxyServer,
		secureProxyServer:      secureProxyServer,
		dummyProxyServer:       dummyProxyServer,
		dummySecureProxyServer: secureDummyProxyServer,
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
		go func() {
			err := s.dummySecureProxyServer.ListenAndServeTLS("", "")
			if err != nil {
				panic(err)
			}
		}()
	}

	go func() {
		err := s.secureProxyServer.ListenAndServeTLS("", "")
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
func registerHandlers(c *mux.Router, cfg *config.YurtHubConfiguration, certificateMgr certificate.YurtCertificateManager, rest *rest.RestConfigManager) {
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

	// register handler for ota upgrade
	c.Handle("/pods", ota.GetPods(cfg.StorageWrapper)).Methods("GET")
	c.Handle("/openyurt.io/v1/namespaces/{ns}/pods/{podname}/upgrade",
		ota.HealthyCheck(rest, cfg.NodeName, ota.UpdatePod)).Methods("POST")
}

// healthz returns ok for healthz request
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// GenUseCertMgrAndTLSConfig create a certificate manager for the yurthub server and generate a TLS configuration
func GenUseCertMgrAndTLSConfig(certificateMgr certificate.YurtCertificateManager) (*tls.Config, error) {
	// generate the TLS configuration based on the latest certificate
	rootCert, err := certmanager.GenCertPoolUseCA(certificateMgr.GetCaFile())
	if err != nil {
		klog.Errorf("could not generate a x509 CertPool based on the given CA file, %v", err)
		return nil, err
	}
	tlsCfg, err := certmanager.GenTLSConfigUseCurrentCertAndCertPool(certificateMgr.GetHubServerCert, rootCert, "server")
	if err != nil {
		return nil, err
	}

	return tlsCfg, nil
}
