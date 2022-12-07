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

package util

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

// RemoteProxy is an reverse proxy for remote server
type RemoteProxy struct {
	reverseProxy         *httputil.ReverseProxy
	remoteServer         *url.URL
	currentTransport     http.RoundTripper
	bearerTransport      http.RoundTripper
	upgradeHandler       *proxy.UpgradeAwareHandler
	bearerUpgradeHandler *proxy.UpgradeAwareHandler
	stopCh               <-chan struct{}
}

type responder struct{}

func (r *responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	klog.Errorf("failed while proxying request %s, %v", req.URL, err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// NewRemoteProxy creates an *RemoteProxy object, and will be used by LoadBalancer
func NewRemoteProxy(remoteServer *url.URL,
	modifyResponse func(*http.Response) error,
	errhandler func(http.ResponseWriter, *http.Request, error),
	transportMgr transport.Interface,
	stopCh <-chan struct{}) (*RemoteProxy, error) {
	currentTransport := transportMgr.CurrentTransport()
	if currentTransport == nil {
		return nil, fmt.Errorf("could not get current transport when init proxy backend(%s)", remoteServer.String())
	}
	bearerTransport := transportMgr.BearerTransport()
	if bearerTransport == nil {
		return nil, fmt.Errorf("could not get bearer transport when init proxy backend(%s)", remoteServer.String())
	}

	upgradeAwareHandler := proxy.NewUpgradeAwareHandler(remoteServer, currentTransport, false, true, &responder{})
	upgradeAwareHandler.UseRequestLocation = true
	bearerUpgradeAwareHandler := proxy.NewUpgradeAwareHandler(remoteServer, bearerTransport, false, true, &responder{})
	bearerUpgradeAwareHandler.UseRequestLocation = true

	proxyBackend := &RemoteProxy{
		reverseProxy:         httputil.NewSingleHostReverseProxy(remoteServer),
		remoteServer:         remoteServer,
		currentTransport:     currentTransport,
		bearerTransport:      bearerTransport,
		upgradeHandler:       upgradeAwareHandler,
		bearerUpgradeHandler: bearerUpgradeAwareHandler,
		stopCh:               stopCh,
	}

	proxyBackend.reverseProxy.Transport = proxyBackend
	proxyBackend.reverseProxy.ModifyResponse = modifyResponse
	proxyBackend.reverseProxy.FlushInterval = -1
	proxyBackend.reverseProxy.ErrorHandler = errhandler

	return proxyBackend, nil
}

// Name represents the address of remote server
func (rp *RemoteProxy) Name() string {
	return rp.remoteServer.String()
}

func (rp *RemoteProxy) RemoteServer() *url.URL {
	return rp.remoteServer
}

func (rp *RemoteProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if httpstream.IsUpgradeRequest(req) {
		klog.V(5).Infof("get upgrade request %s", req.URL)
		if isBearerRequest(req) {
			rp.bearerUpgradeHandler.ServeHTTP(rw, req)
		} else {
			rp.upgradeHandler.ServeHTTP(rw, req)
		}
		return
	}

	rp.reverseProxy.ServeHTTP(rw, req)
}

// RoundTrip is used to implement http.RoundTripper for RemoteProxy.
func (rp *RemoteProxy) RoundTrip(req *http.Request) (*http.Response, error) {
	// when edge client(like kube-proxy, flannel, etc) use service account(default InClusterConfig) to access yurthub,
	// Authorization header will be set in request. and when edge client(like kubelet) use x509 certificate to access
	// yurthub, Authorization header in request will be empty.
	if isBearerRequest(req) {
		return rp.bearerTransport.RoundTrip(req)
	}

	return rp.currentTransport.RoundTrip(req)
}

func isBearerRequest(req *http.Request) bool {
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if auth != "" {
		parts := strings.Split(auth, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			klog.V(5).Infof("request: %s with bearer token: %s", hubutil.ReqString(req), parts[1])
			return true
		}
	}
	return false
}
