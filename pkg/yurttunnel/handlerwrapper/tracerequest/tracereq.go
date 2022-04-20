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

package tracerequest

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	hw "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/server/metrics"
)

// TraceReqMiddleware prints request information when start/stop
// handling the request
type traceReqMiddleware struct {
	nodeLister      corelisters.NodeLister
	podLister       corelisters.PodLister
	informersSynced []cache.InformerSynced
}

// NewTraceReqMiddleware returns an middleware object
func NewTraceReqMiddleware() hw.Middleware {
	return &traceReqMiddleware{
		informersSynced: make([]cache.InformerSynced, 0),
	}
}

func (trm *traceReqMiddleware) Name() string {
	return "TraceReqMiddleware"
}

// SetSharedInformerFactory set nodeLister and nodeSynced for WrapHandler
func (trm *traceReqMiddleware) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	trm.nodeLister = factory.Core().V1().Nodes().Lister()
	trm.podLister = factory.Core().V1().Pods().Lister()
	trm.informersSynced = append(trm.informersSynced, factory.Core().V1().Nodes().Informer().HasSynced)
	trm.informersSynced = append(trm.informersSynced, factory.Core().V1().Pods().Informer().HasSynced)
	return nil
}

func (trm *traceReqMiddleware) WrapHandler(handler http.Handler) http.Handler {
	klog.Infof("%d informer synced in traceReqMiddleware", len(trm.informersSynced))
	if !cache.WaitForCacheSync(wait.NeverStop, trm.informersSynced...) {
		klog.Error("failed to sync node cache for trace request middleware")
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		klog.V(3).Infof("request header in traceReqMiddleware: %v with host: %s and urL: %s", req.Header, req.Host, req.URL.String())
		scheme := "https"
		if req.TLS == nil {
			scheme = "http"
		}

		req.URL.Scheme = scheme
		req.URL.Host = req.Host

		host, port, err := net.SplitHostPort(req.Host)
		if err != nil {
			klog.Errorf("request host(%s) is invalid, %v", req.Host, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		//TODO hack code as https://github.com/kubernetes/enhancements/issues/1558
		if execHost, execPort, isExec := trm.hackExecReq(req); isExec {
			host = execHost
			port = execPort
		}
		klog.V(3).Infof("traceReqMiddleware after hackExec host: %s and port %s", host, port)

		// host for accessing edge component(like kubelet) is hostname, not node ip.
		if ip := net.ParseIP(host); ip == nil {
			// 1. transform hostname to nodeIP for request in order to send request to nodeIP address at tunnel-agent
			// 2. put hostname into X-Tunnel-Proxy-Host request header in order to select the correct backend agent.
			if err := trm.modifyRequest(req, host, port); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			// access tunnel-server directly with specified ProxyDestHeaderKey,
			// request's Host should be modified to specified destination.
			proxyDest := req.Header.Get(constants.ProxyDestHeaderKey)
			if len(proxyDest) != 0 {
				destHost, destPort, err := net.SplitHostPort(proxyDest)
				if err == nil && (destHost != host || destPort != port) {
					req.Host = proxyDest
					req.Header.Set("Host", proxyDest)
					req.URL.Host = proxyDest
				}
			}
		}

		// observe metrics
		metrics.Metrics.IncInFlightRequests(req.Method, req.URL.Path)
		defer metrics.Metrics.DecInFlightRequests(req.Method, req.URL.Path)

		klog.V(2).Infof("start handling request %s %s, from %s to %s",
			req.Method, req.URL.String(), req.RemoteAddr, req.Host)

		start := time.Now()
		handler.ServeHTTP(w, req)
		klog.V(2).Infof("stop handling request %s %s, request handling lasts %v",
			req.Method, req.URL.String(), time.Now().Sub(start))
	})
}

// modifyRequest transform hostname to node ip in request and
// add X-Tunnel-Proxy-Host header in request if not set
func (trm *traceReqMiddleware) modifyRequest(req *http.Request, host, port string) error {
	node, err := trm.nodeLister.Get(host)
	if err != nil {
		klog.Errorf("failed to get node(%s), %v", host, err)
		return err
	}

	nodeIP := getNodeIP(node)
	if nodeIP == "" {
		klog.Errorf("failed to get node(%s) ip", host)
		return errors.New("failed to get node ip")
	}

	// transform hostname to node ip in request
	proxyDest := net.JoinHostPort(nodeIP, port)
	req.Host = proxyDest
	req.Header.Set("Host", proxyDest)
	req.URL.Host = proxyDest

	// add X-Tunnel-Proxy-Host header in request
	if len(req.Header.Get(constants.ProxyHostHeaderKey)) == 0 {
		req.Header.Set(constants.ProxyHostHeaderKey, host)
	}
	return nil
}

// getNodeIP get internal ip for node
func getNodeIP(node *corev1.Node) string {
	var nodeIP string
	if node != nil {
		for _, nodeAddr := range node.Status.Addresses {
			if nodeAddr.Type == corev1.NodeInternalIP {
				nodeIP = nodeAddr.Address
				break
			}
		}
	}
	return nodeIP
}

// for `kubectl exec` request, get pod's node name and proxy the request to the node
func (trm *traceReqMiddleware) hackExecReq(req *http.Request) (host string, port string, isExec bool) {

	if !strings.HasPrefix(req.URL.Path, "/exec") {
		return "", "", false
	}

	paths := strings.Split(req.URL.Path, "/")

	if len(paths) == 5 && paths[1] == "exec" {
		isUpgradeRequest := false
		connHeaderVals := req.Header.Values("Connection")
		for _, tmp := range connHeaderVals {
			if "Upgrade" == tmp {
				isUpgradeRequest = true
			}
		}
		if isUpgradeRequest {
			if execPod, err := trm.podLister.Pods(paths[2]).Get(paths[3]); err == nil {
				klog.Infof("podname: %s", execPod.Name)
				if node, nerr := trm.nodeLister.Get(execPod.Spec.NodeName); nerr == nil {
					klog.Infof("node: %v", node.Status.DaemonEndpoints.KubeletEndpoint.Port)
					return execPod.Spec.NodeName, strconv.Itoa(int(node.Status.DaemonEndpoints.KubeletEndpoint.Port)), true
				} else {
					klog.Errorf("hacexec %v", nerr)
				}
			} else {
				klog.Errorf("hacexec %v", err)
			}
		}

	}
	return "", "", false
}
