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

package servicetopology

import (
	"context"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

const (
	// FilterName filter is used to reassemble endpointslice in order to make the service traffic
	// under the topology that defined by service.Annotation["openyurt.io/topologyKeys"]
	FilterName = "servicetopology"

	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNode     = "kubernetes.io/hostname"
	AnnotationServiceTopologyValueZone     = "kubernetes.io/zone"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"
)

// Register registers a filter
func Register(filters *base.Filters) {
	filters.Register(FilterName, func() (filter.ObjectFilter, error) {
		return NewServiceTopologyFilter()
	})
}

func NewServiceTopologyFilter() (filter.ObjectFilter, error) {
	return &serviceTopologyFilter{}, nil
}

type serviceTopologyFilter struct {
	serviceLister      listers.ServiceLister
	serviceSynced      cache.InformerSynced
	enablePoolTopology bool
	nodesGetter        filter.NodesInPoolGetter
	nodesSynced        cache.InformerSynced
	nodePoolName       string
	nodeName           string
	client             kubernetes.Interface
}

func (stf *serviceTopologyFilter) Name() string {
	return FilterName
}

func (stf *serviceTopologyFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"endpoints":      sets.NewString("list", "watch"),
		"endpointslices": sets.NewString("list", "watch"),
	}
}

func (stf *serviceTopologyFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	stf.serviceLister = factory.Core().V1().Services().Lister()
	stf.serviceSynced = factory.Core().V1().Services().Informer().HasSynced

	return nil
}

func (stf *serviceTopologyFilter) SetNodesGetterAndSynced(nodesGetter filter.NodesInPoolGetter, nodesSynced cache.InformerSynced, enablePoolTopology bool) error {
	stf.nodesGetter = nodesGetter
	stf.nodesSynced = nodesSynced
	stf.enablePoolTopology = enablePoolTopology
	return nil
}

func (stf *serviceTopologyFilter) SetNodeName(nodeName string) error {
	stf.nodeName = nodeName

	return nil
}

func (stf *serviceTopologyFilter) SetNodePoolName(poolName string) error {
	stf.nodePoolName = poolName
	return nil
}

func (stf *serviceTopologyFilter) SetKubeClient(client kubernetes.Interface) error {
	stf.client = client
	return nil
}

func (stf *serviceTopologyFilter) resolveNodePoolName() string {
	if len(stf.nodePoolName) != 0 {
		return stf.nodePoolName
	}

	node, err := stf.client.CoreV1().Nodes().Get(context.Background(), stf.nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("could not get node(%s) in serviceTopologyFilter filter, %v", stf.nodeName, err)
		return stf.nodePoolName
	}
	stf.nodePoolName = node.Labels[projectinfo.GetNodePoolLabel()]
	return stf.nodePoolName
}

func (stf *serviceTopologyFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	if ok := cache.WaitForCacheSync(stopCh, stf.serviceSynced, stf.nodesSynced); !ok {
		return obj
	}

	switch v := obj.(type) {
	case *v1.Endpoints, *discoveryV1beta1.EndpointSlice, *discovery.EndpointSlice:
		return stf.serviceTopologyHandler(v)
	default:
		return obj
	}
}

func (stf *serviceTopologyFilter) serviceTopologyHandler(obj runtime.Object) runtime.Object {
	serviceTopologyType := stf.resolveServiceTopologyType(obj)
	if len(serviceTopologyType) == 0 {
		return obj
	}

	switch serviceTopologyType {
	case AnnotationServiceTopologyValueNode:
		// close traffic on the same node
		return stf.nodeTopologyHandler(obj)
	case AnnotationServiceTopologyValueNodePool, AnnotationServiceTopologyValueZone:
		// close traffic on the same node pool
		if stf.enablePoolTopology {
			return stf.nodePoolTopologyHandler(obj)
		}
		return obj
	default:
		return obj
	}
}

func (stf *serviceTopologyFilter) resolveServiceTopologyType(obj runtime.Object) string {
	var svcNamespace, svcName string
	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		svcNamespace = v.Namespace
		svcName = v.Labels[discoveryV1beta1.LabelServiceName]
	case *discovery.EndpointSlice:
		svcNamespace = v.Namespace
		svcName = v.Labels[discovery.LabelServiceName]
	case *v1.Endpoints:
		svcNamespace = v.Namespace
		svcName = v.Name
	default:
		return ""
	}

	svc, err := stf.serviceLister.Services(svcNamespace).Get(svcName)
	if err != nil {
		klog.Warningf("serviceTopologyFilterHandler: could not get service %s/%s, err: %v", svcNamespace, svcName, err)
		return ""
	}

	if svc.Annotations != nil {
		return svc.Annotations[AnnotationServiceTopologyKey]
	}
	return ""
}

func (stf *serviceTopologyFilter) nodeTopologyHandler(obj runtime.Object) runtime.Object {
	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		return reassembleV1beta1EndpointSlice(v, stf.nodeName, nil)
	case *discovery.EndpointSlice:
		return reassembleEndpointSlice(v, stf.nodeName, nil)
	case *v1.Endpoints:
		return reassembleEndpoints(v, stf.nodeName, nil)
	default:
		return obj
	}
}

func (stf *serviceTopologyFilter) nodePoolTopologyHandler(obj runtime.Object) runtime.Object {
	nodePoolName := stf.resolveNodePoolName()
	if len(nodePoolName) == 0 {
		klog.Infof("node(%s) is not added into node pool, so fall into node topology", stf.nodeName)
		return stf.nodeTopologyHandler(obj)
	}

	nodes, err := stf.nodesGetter(nodePoolName)
	if err != nil {
		klog.Warningf("serviceTopologyFilter: could not get nodes for node pool %s, err: %v", nodePoolName, err)
		return obj
	}

	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		return reassembleV1beta1EndpointSlice(v, "", nodes)
	case *discovery.EndpointSlice:
		return reassembleEndpointSlice(v, "", nodes)
	case *v1.Endpoints:
		return reassembleEndpoints(v, "", nodes)
	default:
		return obj
	}
}

// reassembleV1beta1EndpointSlice will discard endpoints that are not on the same node/nodePool for v1beta1.EndpointSlice
func reassembleV1beta1EndpointSlice(endpointSlice *discoveryV1beta1.EndpointSlice, nodeName string, nodes []string) *discoveryV1beta1.EndpointSlice {
	if len(nodeName) != 0 && len(nodes) != 0 {
		klog.Warningf("reassembleV1beta1EndpointSlice: nodeName(%s) and nodes can not be set at the same time", nodeName)
		return endpointSlice
	}

	var newEps []discoveryV1beta1.Endpoint
	for i := range endpointSlice.Endpoints {
		if len(nodeName) != 0 {
			if endpointSlice.Endpoints[i].Topology[v1.LabelHostname] == nodeName {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}

		if len(nodes) != 0 {
			if inSameNodePool(endpointSlice.Endpoints[i].Topology[v1.LabelHostname], nodes) {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}
	}

	// even no endpoints left, empty endpoints slice should be returned
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpointSlice will discard endpoints that are not on the same node/nodePool for v1.EndpointSlice
func reassembleEndpointSlice(endpointSlice *discovery.EndpointSlice, nodeName string, nodes []string) *discovery.EndpointSlice {
	if len(nodeName) != 0 && len(nodes) != 0 {
		klog.Warningf("reassembleEndpointSlice: nodeName(%s) and nodePool can not be set at the same time", nodeName)
		return endpointSlice
	}

	var newEps []discovery.Endpoint
	for i := range endpointSlice.Endpoints {
		if len(nodeName) != 0 {
			if *endpointSlice.Endpoints[i].NodeName == nodeName {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}

		if len(nodes) != 0 {
			if inSameNodePool(*endpointSlice.Endpoints[i].NodeName, nodes) {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}
	}

	// even no endpoints left, empty endpoints slice should be returned
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpoints will discard subset that are not on the same node/nodePool for v1.Endpoints
func reassembleEndpoints(endpoints *v1.Endpoints, nodeName string, nodes []string) *v1.Endpoints {
	if len(nodeName) != 0 && len(nodes) != 0 {
		klog.Warningf("reassembleEndpoints: nodeName(%s) and nodePool can not be set at the same time", nodeName)
		return endpoints
	}

	var newEpSubsets []v1.EndpointSubset
	for i := range endpoints.Subsets {
		if len(nodeName) != 0 {
			endpoints.Subsets[i].Addresses = filterValidEndpointsAddr(endpoints.Subsets[i].Addresses, nodeName, nil)
			endpoints.Subsets[i].NotReadyAddresses = filterValidEndpointsAddr(endpoints.Subsets[i].NotReadyAddresses, nodeName, nil)
		}

		if len(nodes) != 0 {
			endpoints.Subsets[i].Addresses = filterValidEndpointsAddr(endpoints.Subsets[i].Addresses, "", nodes)
			endpoints.Subsets[i].NotReadyAddresses = filterValidEndpointsAddr(endpoints.Subsets[i].NotReadyAddresses, "", nodes)
		}

		if len(endpoints.Subsets[i].Addresses) != 0 || len(endpoints.Subsets[i].NotReadyAddresses) != 0 {
			newEpSubsets = append(newEpSubsets, endpoints.Subsets[i])
		}
	}

	// even no subsets left, empty subset slice should be returned
	endpoints.Subsets = newEpSubsets
	return endpoints
}

func filterValidEndpointsAddr(addresses []v1.EndpointAddress, nodeName string, nodes []string) []v1.EndpointAddress {
	var newEpAddresses []v1.EndpointAddress
	for i := range addresses {
		if addresses[i].NodeName == nil {
			continue
		}

		// filter address on the same node
		if len(nodeName) != 0 {
			if nodeName == *addresses[i].NodeName {
				newEpAddresses = append(newEpAddresses, addresses[i])
			}
		}

		// filter address on the same node pool
		if len(nodes) != 0 {
			if inSameNodePool(*addresses[i].NodeName, nodes) {
				newEpAddresses = append(newEpAddresses, addresses[i])
			}
		}
	}
	return newEpAddresses
}

func inSameNodePool(nodeName string, nodeList []string) bool {
	for _, n := range nodeList {
		if nodeName == n {
			return true
		}
	}
	return false
}
