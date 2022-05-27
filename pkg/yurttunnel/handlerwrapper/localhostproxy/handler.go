/*
Copyright 2021 The OpenYurt Authors.

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

package localhostproxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformer "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	utilip "github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	hw "github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

// localHostProxyMiddleware modify request for requests from cloud can access localhost of edge node.
type localHostProxyMiddleware struct {
	sync.RWMutex
	getNodesByIP       func(nodeIP string) ([]*corev1.Node, error)
	localhostPorts     map[string]struct{}
	nodeInformerSynced cache.InformerSynced
	cmInformerSynced   cache.InformerSynced
	loopbackAddr       string
}

func NewLocalHostProxyMiddleware(isIPv6 bool) hw.Middleware {
	return &localHostProxyMiddleware{
		localhostPorts: make(map[string]struct{}),
		loopbackAddr:   utilip.MustGetLoopbackIP(isIPv6),
	}
}

func (plm *localHostProxyMiddleware) Name() string {
	return "localHostProxyMiddleware"
}

// WrapHandler modify request header and URL for underlying anp to proxy request.
func (plm *localHostProxyMiddleware) WrapHandler(handler http.Handler) http.Handler {
	// wait for nodes and configmaps have synced
	if !cache.WaitForCacheSync(wait.NeverStop, plm.nodeInformerSynced, plm.cmInformerSynced) {
		klog.Error("failed to sync node or configmap cache")
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Check the request port to see if it needs to be forwarded to the node's localhost
		proxyDest := req.Header.Get(constants.ProxyDestHeaderKey)
		if len(proxyDest) != 0 {
			req.Header.Del(constants.ProxyDestHeaderKey)
		} else {
			proxyDest = req.Host
		}

		nodeIP, port, err := net.SplitHostPort(proxyDest)
		if err != nil {
			klog.Errorf("proxy dest(%s) is invalid %v for request: %s", proxyDest, err, req.URL.String())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// port is included in proxy-localhost-ports, so modify request for
		// forwarding the request to access node's localhost.
		plm.RLock()
		_, ok := plm.localhostPorts[port]
		plm.RUnlock()
		if ok {
			// set up X-Tunnel-Proxy-Host header in request for underlying anp to select backend tunnel agent conn.
			if len(req.Header.Get(constants.ProxyHostHeaderKey)) == 0 {
				nodeName, err := plm.resolveNodeNameByNodeIP(nodeIP)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				req.Header.Set(constants.ProxyHostHeaderKey, nodeName)
			}

			proxyDest = net.JoinHostPort(plm.loopbackAddr, port)
			oldHost := req.URL.Host
			req.Host = proxyDest
			req.Header.Set("Host", proxyDest)
			req.URL.Host = proxyDest
			klog.V(2).Infof("proxy request %s to access localhost, changed from %s to %s(%s)", req.URL.String(), oldHost, proxyDest, req.Header.Get(constants.ProxyHostHeaderKey))
		}

		klog.V(3).Infof("request header in localHostProxyMiddleware: %v with host: %s and urL: %s", req.Header, req.Host, req.URL.String())
		handler.ServeHTTP(w, req)
	})
}

// SetSharedInformerFactory init GetNodesByIP and configmap event handler for WrapHandler
func (plm *localHostProxyMiddleware) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	if factory == nil {
		return errors.New("shared informer factory should not be nil")
	}

	nodeInformer := factory.Core().V1().Nodes()
	if err := nodeInformer.Informer().AddIndexers(cache.Indexers{constants.NodeIPKeyIndex: getNodeAddress}); err != nil {
		klog.ErrorS(err, "failed to add statusInternalIP indexer")
		return err
	}

	plm.getNodesByIP = func(nodeIP string) ([]*corev1.Node, error) {
		objs, err := nodeInformer.Informer().GetIndexer().ByIndex(constants.NodeIPKeyIndex, nodeIP)
		if err != nil {
			return nil, err
		}

		nodes := make([]*corev1.Node, 0, len(objs))
		for _, obj := range objs {
			if node, ok := obj.(*corev1.Node); ok {
				nodes = append(nodes, node)
			}
		}

		return nodes, nil
	}
	plm.nodeInformerSynced = nodeInformer.Informer().HasSynced

	cmInformer := factory.InformerFor(&corev1.ConfigMap{}, newConfigMapInformer)
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    plm.addConfigMap,
		UpdateFunc: plm.updateConfigMap,
	})
	plm.cmInformerSynced = cmInformer.HasSynced

	return nil
}

// newConfigMapInformer creates a shared index informer that returns only interested configmaps
func newConfigMapInformer(cs clientset.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	selector := fmt.Sprintf("metadata.name=%v", util.YurttunnelServerDnatConfigMapName)
	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = selector
	}
	return coreinformer.NewFilteredConfigMapInformer(cs, util.YurttunnelServerDnatConfigMapNs, resyncPeriod, nil, tweakListOptions)
}

// addConfigMap handle configmap add event
func (plm *localHostProxyMiddleware) addConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.DeletionTimestamp != nil {
		return
	}
	klog.V(2).Infof("handle configmap add event for %v/%v to update localhost ports", cm.Namespace, cm.Name)
	plm.replaceLocalHostPorts(cm.Data[util.YurtTunnelLocalHostProxyPorts])
}

// updateConfigMap handle configmap update event
func (plm *localHostProxyMiddleware) updateConfigMap(oldObj, newObj interface{}) {
	oldConfigMap, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	newConfigMap, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	if oldConfigMap.Data[util.YurtTunnelLocalHostProxyPorts] == newConfigMap.Data[util.YurtTunnelLocalHostProxyPorts] {
		return
	}

	klog.V(2).Infof("handle configmap update event for %v/%v to update localhost ports", newConfigMap.Namespace, newConfigMap.Name)
	plm.replaceLocalHostPorts(newConfigMap.Data[util.YurtTunnelLocalHostProxyPorts])
}

// replaceLocalHostPorts replace all localhost ports by new specified ports.
func (plm *localHostProxyMiddleware) replaceLocalHostPorts(portsStr string) {
	ports := make([]string, 0)
	for _, port := range strings.Split(portsStr, util.PortsSeparator) {
		if len(strings.TrimSpace(port)) != 0 {
			ports = append(ports, strings.TrimSpace(port))
		}
	}

	plm.Lock()
	defer plm.Unlock()
	for port := range plm.localhostPorts {
		delete(plm.localhostPorts, port)
	}

	for i := range ports {
		plm.localhostPorts[ports[i]] = struct{}{}
	}
}

// resolveProxyHostFromRequest get proxy host info from request
func (plm *localHostProxyMiddleware) resolveNodeNameByNodeIP(nodeIP string) (string, error) {
	var nodeName string

	if nodes, err := plm.getNodesByIP(nodeIP); err != nil || len(nodes) == 0 {
		klog.Warningf("failed to get node for node ip(%s)", nodeIP)
		return "", fmt.Errorf("proxy node ip(%s) is not exist in cluster", nodeIP)
	} else if len(nodes) != 1 {
		klog.Warningf("more than one node with the same IP(%s), so unable to proxy request", nodeIP)
		return "", fmt.Errorf("more than one node with ip(%s) in cluster", nodeIP)
	} else {
		nodeName = nodes[0].Name
	}

	if len(nodeName) == 0 {
		klog.Warningf("node name for node ip(%s) is not exist in cluster", nodeIP)
		return "", fmt.Errorf("failed to get node name for node ip(%s)", nodeIP)
	}

	klog.V(5).Infof("resolved node name(%s) for node ip(%s)", nodeName, nodeIP)
	return nodeName, nil
}

// getNodeAddress return the internal ip address of specified node.
func getNodeAddress(obj interface{}) ([]string, error) {
	node, ok := obj.(*corev1.Node)
	if !ok || node == nil {
		return []string{}, nil
	}

	if withoutAgent(node) {
		// node has no running tunnel agent, do not go through tunnel server
		return []string{}, nil
	}

	for _, nodeAddr := range node.Status.Addresses {
		if nodeAddr.Type == corev1.NodeInternalIP {
			return []string{nodeAddr.Address}, nil
		}
	}

	return []string{}, nil
}

// withoutAgent used to determine whether the node is running an tunnel agent
func withoutAgent(node *corev1.Node) bool {
	tunnelAgentNode, ok := node.Labels[projectinfo.GetEdgeEnableTunnelLabelKey()]
	if ok && tunnelAgentNode == "true" {
		return false
	}

	edgeNode, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
	if ok && edgeNode == "true" {
		return false
	}
	return true
}
