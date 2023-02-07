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
	"fmt"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	discoveryV1beta1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
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

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.ServiceTopologyFilterName, func() (filter.ObjectFilter, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *serviceTopologyFilter {
	return &serviceTopologyFilter{
		workingMode: util.WorkingModeEdge,
	}
}

type serviceTopologyFilter struct {
	serviceLister  listers.ServiceLister
	serviceSynced  cache.InformerSynced
	nodePoolLister appslisters.NodePoolLister
	nodePoolSynced cache.InformerSynced
	nodeGetter     filter.NodeGetter
	nodeSynced     cache.InformerSynced
	nodeName       string
	workingMode    util.WorkingMode
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

func (stf *serviceTopologyFilter) SetWorkingMode(mode util.WorkingMode) error {
	stf.workingMode = mode
	return nil
}

func (stf *serviceTopologyFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	stf.serviceLister = factory.Core().V1().Services().Lister()
	stf.serviceSynced = factory.Core().V1().Services().Informer().HasSynced

	if stf.workingMode == util.WorkingModeCloud {
		klog.Infof("prepare list/watch to sync node(%s) for cloud working mode", stf.nodeName)
		stf.nodeSynced = factory.Core().V1().Nodes().Informer().HasSynced
		stf.nodeGetter = factory.Core().V1().Nodes().Lister().Get
	}

	return nil
}

func (stf *serviceTopologyFilter) SetYurtSharedInformerFactory(yurtFactory yurtinformers.SharedInformerFactory) error {
	stf.nodePoolLister = yurtFactory.Apps().V1alpha1().NodePools().Lister()
	stf.nodePoolSynced = yurtFactory.Apps().V1alpha1().NodePools().Informer().HasSynced

	return nil
}

func (stf *serviceTopologyFilter) SetNodeName(nodeName string) error {
	stf.nodeName = nodeName

	return nil
}

// TODO: should use disk storage as parameter instead of StorageWrapper
// we can internally construct a new StorageWrapper with passed-in disk storage
func (stf *serviceTopologyFilter) SetStorageWrapper(s cachemanager.StorageWrapper) error {
	if s.Name() != disk.StorageName {
		return fmt.Errorf("serviceTopologyFilter can only support disk storage currently, cannot use %s", s.Name())
	}

	if len(stf.nodeName) == 0 {
		return fmt.Errorf("node name for serviceTopologyFilter is not ready")
	}

	// hub agent will list/watch node from kube-apiserver when hub agent work as cloud mode
	if stf.workingMode == util.WorkingModeCloud {
		return nil
	}
	klog.Infof("prepare local disk storage to sync node(%s) for edge working mode", stf.nodeName)

	nodeKey, err := s.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Name:      stf.nodeName,
		Resources: "nodes",
		Group:     "",
		Version:   "v1",
	})
	if err != nil {
		return fmt.Errorf("failed to get node key for %s, %v", stf.nodeName, err)
	}
	stf.nodeSynced = func() bool {
		obj, err := s.Get(nodeKey)
		if err != nil || obj == nil {
			return false
		}

		if _, ok := obj.(*v1.Node); !ok {
			return false
		}

		return true
	}

	stf.nodeGetter = func(name string) (*v1.Node, error) {
		key, err := s.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Name:      name,
			Resources: "nodes",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			return nil, fmt.Errorf("nodeGetter failed to get node key for %s, %v", name, err)
		}
		obj, err := s.Get(key)
		if err != nil {
			return nil, err
		} else if obj == nil {
			return nil, fmt.Errorf("node(%s) is not ready", name)
		}

		if node, ok := obj.(*v1.Node); ok {
			return node, nil
		}

		return nil, fmt.Errorf("node(%s) is not found", name)
	}

	return nil
}

func (stf *serviceTopologyFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	if ok := cache.WaitForCacheSync(stopCh, stf.nodeSynced, stf.serviceSynced, stf.nodePoolSynced); !ok {
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
		klog.Warningf("serviceTopologyFilterHandler: failed to get service %s/%s, err: %v", svcNamespace, svcName, err)
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
	currentNode, err := stf.nodeGetter(stf.nodeName)
	if err != nil {
		klog.Warningf("skip serviceTopologyFilterHandler, failed to get current node %s, err: %v", stf.nodeName, err)
		return obj
	}

	nodePoolName, ok := currentNode.Labels[nodepoolv1alpha1.LabelCurrentNodePool]
	if !ok || len(nodePoolName) == 0 {
		klog.Infof("node(%s) is not added into node pool, so fall into node topology", stf.nodeName)
		return stf.nodeTopologyHandler(obj)
	}

	nodePool, err := stf.nodePoolLister.Get(nodePoolName)
	if err != nil {
		klog.Warningf("serviceTopologyFilterHandler: failed to get nodepool %s, err: %v", nodePoolName, err)
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

	// even no endpoints left, empty endpoints slice should be returned
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

	// even no endpoints left, empty endpoints slice should be returned
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

	// even no subsets left, empty subset slice should be returned
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
