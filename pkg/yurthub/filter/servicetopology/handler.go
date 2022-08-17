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
	"io"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	appslisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
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

type serviceTopologyFilterHandler struct {
	nodeName       string
	serializer     *serializer.Serializer
	serviceLister  listers.ServiceLister
	nodePoolLister appslisters.NodePoolLister
	nodeGetter     filter.NodeGetter
}

func NewServiceTopologyFilterHandler(
	nodeName string,
	serializer *serializer.Serializer,
	serviceLister listers.ServiceLister,
	nodePoolLister appslisters.NodePoolLister,
	nodeGetter filter.NodeGetter) filter.Handler {
	return &serviceTopologyFilterHandler{
		nodeName:       nodeName,
		serializer:     serializer,
		serviceLister:  serviceLister,
		nodePoolLister: nodePoolLister,
		nodeGetter:     nodeGetter,
	}
}

//ObjectResponseFilter filter the endpointSlice or endpoints from get response object and return the bytes
func (fh *serviceTopologyFilterHandler) ObjectResponseFilter(b []byte) ([]byte, error) {
	obj, err := fh.serializer.Decode(b)
	if err != nil || obj == nil {
		klog.Errorf("skip serviceTopologyFilterHandler: failed to decode response in ObjectResponseFilter, %v", err)
		return b, nil
	}

	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSliceList:
		// filter endpointSlice before k8s 1.21
		var items []discoveryV1beta1.EndpointSlice
		for i := range v.Items {
			isNil, item := fh.serviceTopologyHandler(&v.Items[i])
			if !isNil {
				eps := item.(*discoveryV1beta1.EndpointSlice)
				items = append(items, *eps)
			}
		}
		v.Items = items
		return fh.serializer.Encode(v)
	case *discovery.EndpointSliceList:
		var items []discovery.EndpointSlice
		for i := range v.Items {
			isNil, item := fh.serviceTopologyHandler(&v.Items[i])
			if !isNil {
				eps := item.(*discovery.EndpointSlice)
				items = append(items, *eps)
			}
		}
		v.Items = items
		return fh.serializer.Encode(v)
	case *v1.EndpointsList:
		var items []v1.Endpoints
		for i := range v.Items {
			isNil, item := fh.serviceTopologyHandler(&v.Items[i])
			if !isNil {
				ep := item.(*v1.Endpoints)
				items = append(items, *ep)
			}
		}
		v.Items = items
		return fh.serializer.Encode(v)
	default:
		return b, nil
	}
}

// StreamResponseFilter filter the endpointslice or endpoints from watch response object and return the bytes
func (fh *serviceTopologyFilterHandler) StreamResponseFilter(rc io.ReadCloser, ch chan watch.Event) error {
	defer func() {
		close(ch)
	}()

	d, err := fh.serializer.WatchDecoder(rc)
	if err != nil {
		klog.Errorf("StreamResponseFilter of serviceTopologyFilterHandler ended with error, %v", err)
		return err
	}

	for {
		watchType, obj, err := d.Decode()
		if err != nil {
			return err
		}

		isNil, filteredObj := fh.serviceTopologyHandler(obj)
		if !isNil {
			ch <- watch.Event{
				Type:   watchType,
				Object: filteredObj,
			}
		}
	}
}

func (fh *serviceTopologyFilterHandler) serviceTopologyHandler(obj runtime.Object) (bool, runtime.Object) {
	needHandle, serviceTopologyType := fh.resolveServiceTopologyType(obj)
	if !needHandle || len(serviceTopologyType) == 0 {
		return false, obj
	}

	switch serviceTopologyType {
	case AnnotationServiceTopologyValueNode:
		// close traffic on the same node
		return fh.nodeTopologyHandler(obj)
	case AnnotationServiceTopologyValueNodePool, AnnotationServiceTopologyValueZone:
		// close traffic on the same node pool
		return fh.nodePoolTopologyHandler(obj)
	default:
		return false, obj
	}
}

func (fh *serviceTopologyFilterHandler) resolveServiceTopologyType(obj runtime.Object) (bool, string) {
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

	svc, err := fh.serviceLister.Services(svcNamespace).Get(svcName)
	if err != nil {
		klog.Infof("serviceTopologyFilterHandler: failed to get service %s/%s, err: %v", svcNamespace, svcName, err)
		return false, ""
	}

	if topologyValueSets.Has(svc.Annotations[AnnotationServiceTopologyKey]) {
		return true, svc.Annotations[AnnotationServiceTopologyKey]
	}
	return false, ""
}

func (fh *serviceTopologyFilterHandler) nodeTopologyHandler(obj runtime.Object) (bool, runtime.Object) {
	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		newObj := reassembleV1beta1EndpointSlice(v, fh.nodeName, nil)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	case *discovery.EndpointSlice:
		newObj := reassembleEndpointSlice(v, fh.nodeName, nil)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	case *v1.Endpoints:
		newObj := reassembleEndpoints(v, fh.nodeName, nil)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	default:
		return false, obj
	}
}

func (fh *serviceTopologyFilterHandler) nodePoolTopologyHandler(obj runtime.Object) (bool, runtime.Object) {
	currentNode, err := fh.nodeGetter(fh.nodeName)
	if err != nil {
		klog.Infof("skip serviceTopologyFilterHandler, failed to get current node %s, err: %v", fh.nodeName, err)
		return false, obj
	}

	nodePoolName, ok := currentNode.Labels[nodepoolv1alpha1.LabelCurrentNodePool]
	if !ok || len(nodePoolName) == 0 {
		klog.Infof("node(%s) is not added into node pool, so fall into node topology", fh.nodeName)
		return fh.nodeTopologyHandler(obj)
	}

	nodePool, err := fh.nodePoolLister.Get(nodePoolName)
	if err != nil {
		klog.Infof("serviceTopologyFilterHandler: failed to get nodepool %s, err: %v", nodePoolName, err)
		return false, obj
	}

	switch v := obj.(type) {
	case *discoveryV1beta1.EndpointSlice:
		newObj := reassembleV1beta1EndpointSlice(v, "", nodePool)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	case *discovery.EndpointSlice:
		newObj := reassembleEndpointSlice(v, "", nodePool)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	case *v1.Endpoints:
		newObj := reassembleEndpoints(v, "", nodePool)
		if newObj == nil {
			return true, obj
		}
		return false, newObj
	default:
		return false, obj
	}
}

// reassembleV1beta1EndpointSlice will discard endpoints that are not on the same node/nodePool for v1beta1.EndpointSlice
func reassembleV1beta1EndpointSlice(endpointSlice *discoveryV1beta1.EndpointSlice, nodeName string, nodePool *nodepoolv1alpha1.NodePool) *discoveryV1beta1.EndpointSlice {
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
	if len(newEps) == 0 {
		return nil
	}
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpointSlice will discard endpoints that are not on the same node/nodePool for v1.EndpointSlice
func reassembleEndpointSlice(endpointSlice *discovery.EndpointSlice, nodeName string, nodePool *nodepoolv1alpha1.NodePool) *discovery.EndpointSlice {
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

	if len(newEps) == 0 {
		return nil
	}
	endpointSlice.Endpoints = newEps
	return endpointSlice
}

// reassembleEndpoints will discard subset that are not on the same node/nodePool for v1.Endpoints
func reassembleEndpoints(endpoints *v1.Endpoints, nodeName string, nodePool *nodepoolv1alpha1.NodePool) *v1.Endpoints {
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
	if len(newEpSubsets) == 0 {
		// this endpoints has no valid addresses for ingress controller, return nil to ignore it
		return nil
	}
	endpoints.Subsets = newEpSubsets
	return endpoints
}

func filterValidEndpointsAddr(addresses []v1.EndpointAddress, nodeName string, nodePool *nodepoolv1alpha1.NodePool) []v1.EndpointAddress {
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
