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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNode     = "kubernetes.io/hostname"
	AnnotationServiceTopologyValueZone     = "kubernetes.io/zone"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"
)

var (
	topologyValueSets = sets.NewString(AnnotationServiceTopologyValueNode, AnnotationServiceTopologyValueZone, AnnotationServiceTopologyValueNodePool)
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.ServiceTopologyFilterName, func() (filter.ObjectFilter, error) {
		return NewServiceTopologyFilter()
	})
}

func NewServiceTopologyFilter() (filter.ObjectFilter, error) {
	return &serviceTopologyFilter{}, nil
}

type serviceTopologyFilter struct {
	serviceLister  listers.ServiceLister
	serviceSynced  cache.InformerSynced
	nodePoolLister cache.GenericLister
	nodePoolSynced cache.InformerSynced
	nodePoolName   string
	nodeName       string
	client         kubernetes.Interface
}

func (stf *serviceTopologyFilter) Name() string {
	return filter.ServiceTopologyFilterName
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

func (stf *serviceTopologyFilter) SetNodePoolInformerFactory(dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) error {
	gvr := v1beta1.GroupVersion.WithResource("nodepools")
	stf.nodePoolLister = dynamicInformerFactory.ForResource(gvr).Lister()
	stf.nodePoolSynced = dynamicInformerFactory.ForResource(gvr).Informer().HasSynced

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
	if ok := cache.WaitForCacheSync(stopCh, stf.serviceSynced, stf.nodePoolSynced); !ok {
		return obj
	}

	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSliceList:
		// filter endpointSlice before k8s 1.21
		var items []discoveryV1beta1.EndpointSlice
		for i := range v.Items {
			eps := stf.serviceTopologyHandler(&v.Items[i]).(*discoveryV1beta1.EndpointSlice)
			items = append(items, *eps)
		}
		v.Items = items
		return v
	case *discovery.EndpointSliceList:
		var items []discovery.EndpointSlice
		for i := range v.Items {
			eps := stf.serviceTopologyHandler(&v.Items[i]).(*discovery.EndpointSlice)
			items = append(items, *eps)
		}
		v.Items = items
		return v
	case *v1.EndpointsList:
		var items []v1.Endpoints
		for i := range v.Items {
			ep := stf.serviceTopologyHandler(&v.Items[i]).(*v1.Endpoints)
			items = append(items, *ep)
		}
		v.Items = items
		return v
	case *v1.Endpoints, *discoveryV1beta1.EndpointSlice, *discovery.EndpointSlice:
		return stf.serviceTopologyHandler(v)
	default:
		return obj
	}
}

func (stf *serviceTopologyFilter) serviceTopologyHandler(obj runtime.Object) runtime.Object {
	needHandle, serviceTopologyType := stf.resolveServiceTopologyType(obj)
	if !needHandle || len(serviceTopologyType) == 0 {
		return obj
	}

	switch serviceTopologyType {
	case AnnotationServiceTopologyValueNode:
		// close traffic on the same node
		return stf.nodeTopologyHandler(obj)
	case AnnotationServiceTopologyValueNodePool, AnnotationServiceTopologyValueZone:
		// close traffic on the same node pool
		return stf.nodePoolTopologyHandler(obj)
	default:
		return obj
	}
}

func (stf *serviceTopologyFilter) resolveServiceTopologyType(obj runtime.Object) (bool, string) {
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
		return false, ""
	}

	svc, err := stf.serviceLister.Services(svcNamespace).Get(svcName)
	if err != nil {
		klog.Warningf("serviceTopologyFilterHandler: could not get service %s/%s, err: %v", svcNamespace, svcName, err)
		return false, ""
	}

	if topologyValueSets.Has(svc.Annotations[AnnotationServiceTopologyKey]) {
		return true, svc.Annotations[AnnotationServiceTopologyKey]
	}
	return false, ""
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

	runtimeObj, err := stf.nodePoolLister.Get(nodePoolName)
	if err != nil {
		klog.Warningf("serviceTopologyFilterHandler: could not get nodepool %s, err: %v", nodePoolName, err)
		return obj
	}
	var nodePool *v1beta1.NodePool
	switch poolObj := runtimeObj.(type) {
	case *v1beta1.NodePool:
		nodePool = poolObj
	case *unstructured.Unstructured:
		nodePool = new(v1beta1.NodePool)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(poolObj.UnstructuredContent(), nodePool); err != nil {
			klog.Warningf("object(%#+v) is not a v1beta1.NodePool", poolObj)
			return obj
		}
	default:
		klog.Warningf("object(%#+v) is not a unknown type", poolObj)
		return obj
	}

	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		return reassembleV1beta1EndpointSlice(v, "", nodePool)
	case *discovery.EndpointSlice:
		return reassembleEndpointSlice(v, "", nodePool)
	case *v1.Endpoints:
		return reassembleEndpoints(v, "", nodePool)
	default:
		return obj
	}
}

// reassembleV1beta1EndpointSlice will discard endpoints that are not on the same node/nodePool for v1beta1.EndpointSlice
func reassembleV1beta1EndpointSlice(endpointSlice *discoveryV1beta1.EndpointSlice, nodeName string, nodePool *v1beta1.NodePool) *discoveryV1beta1.EndpointSlice {
	if len(nodeName) != 0 && nodePool != nil {
		klog.Warningf("reassembleV1beta1EndpointSlice: nodeName(%s) and nodePool can not be set at the same time", nodeName)
		return endpointSlice
	}

	var newEps []discoveryV1beta1.Endpoint
	for i := range endpointSlice.Endpoints {
		if len(nodeName) != 0 {
			if endpointSlice.Endpoints[i].Topology[v1.LabelHostname] == nodeName {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}

		if nodePool != nil {
			if inSameNodePool(endpointSlice.Endpoints[i].Topology[v1.LabelHostname], nodePool.Status.Nodes) {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}
	}

	// even no endpoints left, empty endpoints slice should be returned
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpointSlice will discard endpoints that are not on the same node/nodePool for v1.EndpointSlice
func reassembleEndpointSlice(endpointSlice *discovery.EndpointSlice, nodeName string, nodePool *v1beta1.NodePool) *discovery.EndpointSlice {
	if len(nodeName) != 0 && nodePool != nil {
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

		if nodePool != nil {
			if inSameNodePool(*endpointSlice.Endpoints[i].NodeName, nodePool.Status.Nodes) {
				newEps = append(newEps, endpointSlice.Endpoints[i])
			}
		}
	}

	// even no endpoints left, empty endpoints slice should be returned
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpoints will discard subset that are not on the same node/nodePool for v1.Endpoints
func reassembleEndpoints(endpoints *v1.Endpoints, nodeName string, nodePool *v1beta1.NodePool) *v1.Endpoints {
	if len(nodeName) != 0 && nodePool != nil {
		klog.Warningf("reassembleEndpoints: nodeName(%s) and nodePool can not be set at the same time", nodeName)
		return endpoints
	}

	var newEpSubsets []v1.EndpointSubset
	for i := range endpoints.Subsets {
		if len(nodeName) != 0 {
			endpoints.Subsets[i].Addresses = filterValidEndpointsAddr(endpoints.Subsets[i].Addresses, nodeName, nil)
			endpoints.Subsets[i].NotReadyAddresses = filterValidEndpointsAddr(endpoints.Subsets[i].NotReadyAddresses, nodeName, nil)
		}

		if nodePool != nil {
			endpoints.Subsets[i].Addresses = filterValidEndpointsAddr(endpoints.Subsets[i].Addresses, "", nodePool)
			endpoints.Subsets[i].NotReadyAddresses = filterValidEndpointsAddr(endpoints.Subsets[i].NotReadyAddresses, "", nodePool)
		}

		if len(endpoints.Subsets[i].Addresses) != 0 || len(endpoints.Subsets[i].NotReadyAddresses) != 0 {
			newEpSubsets = append(newEpSubsets, endpoints.Subsets[i])
		}
	}

	// even no subsets left, empty subset slice should be returned
	endpoints.Subsets = newEpSubsets
	return endpoints
}

func filterValidEndpointsAddr(addresses []v1.EndpointAddress, nodeName string, nodePool *v1beta1.NodePool) []v1.EndpointAddress {
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
		if nodePool != nil {
			if inSameNodePool(*addresses[i].NodeName, nodePool.Status.Nodes) {
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
