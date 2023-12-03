/*
Copyright 2023 The OpenYurt Authors.

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

package hostnetworkpropagation

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.HostNetworkPropagationFilterName, func() (filter.ObjectFilter, error) {
		return NewHostNetworkPropagationFilter()
	})
}

func NewHostNetworkPropagationFilter() (filter.ObjectFilter, error) {
	return &hostNetworkPropagationFilter{}, nil
}

type hostNetworkPropagationFilter struct {
	nodePoolLister    cache.GenericLister
	nodePoolSynced    cache.InformerSynced
	nodePoolName      string
	client            kubernetes.Interface
	isHostNetworkPool *bool
}

func (hpf *hostNetworkPropagationFilter) Name() string {
	return filter.HostNetworkPropagationFilterName
}

func (hpf *hostNetworkPropagationFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"pods": sets.NewString("list", "watch"),
	}
}

func (hpf *hostNetworkPropagationFilter) SetNodePoolInformerFactory(dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) error {
	gvr := v1beta1.GroupVersion.WithResource("nodepools")
	hpf.nodePoolLister = dynamicInformerFactory.ForResource(gvr).Lister()
	hpf.nodePoolSynced = dynamicInformerFactory.ForResource(gvr).Informer().HasSynced

	return nil
}

func (hpf *hostNetworkPropagationFilter) SetNodePoolName(poolName string) error {
	hpf.nodePoolName = poolName
	return nil
}

func (hpf *hostNetworkPropagationFilter) SetKubeClient(client kubernetes.Interface) error {
	hpf.client = client
	return nil
}

func (hpf *hostNetworkPropagationFilter) resolveNodePoolName(pod *corev1.Pod) string {
	if len(hpf.nodePoolName) != 0 {
		return hpf.nodePoolName
	}

	node, err := hpf.client.CoreV1().Nodes().Get(context.Background(), pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("could not get node(%s) in hostNetworkPropagationFilter, %v", pod.Spec.NodeName, err)
		return hpf.nodePoolName
	}
	hpf.nodePoolName = node.Labels[projectinfo.GetNodePoolLabel()]
	return hpf.nodePoolName
}

func (hpf *hostNetworkPropagationFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj
	}

	// when pod hostnetwork is already set to true, the function will
	// short-circuit and return
	if pod.Spec.HostNetwork {
		return obj
	}

	if hpf.isHostNetworkPool == nil {
		// go to configure IsHostNetworkPool
	} else if *hpf.isHostNetworkPool {
		pod.Spec.HostNetwork = true
		return obj
	} else {
		// nodepool hostNetwork field is false, only short-circuit and return
		return obj
	}

	if ok := cache.WaitForCacheSync(stopCh, hpf.nodePoolSynced); !ok {
		return obj
	}

	nodePoolName := hpf.resolveNodePoolName(pod)
	if len(nodePoolName) == 0 {
		klog.Infof("node(%s) is not added into node pool, so skip hostnetworkpropagation", pod.Spec.NodeName)
		return obj
	}

	runtimeObj, err := hpf.nodePoolLister.Get(nodePoolName)
	if err != nil {
		klog.Warningf("hostNetworkPropagationFilter: could not get nodepool %s, err: %v", nodePoolName, err)
		return obj
	}
	var nodePool *v1beta1.NodePool
	switch poolObj := runtimeObj.(type) {
	case *v1beta1.NodePool:
		nodePool = poolObj
	case *unstructured.Unstructured:
		nodePool = new(v1beta1.NodePool)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(poolObj.UnstructuredContent(), nodePool); err != nil {
			klog.Warningf("hostNetworkPropagationFilter: object(%#+v) is not a v1beta1.NodePool", poolObj)
			return obj
		}
	default:
		klog.Warningf("object(%#+v) is not a unknown type", poolObj)
		return obj
	}

	if nodePool.Spec.HostNetwork {
		valTrue := true
		hpf.isHostNetworkPool = &valTrue
		pod.Spec.HostNetwork = true
	} else {
		valFalse := false
		hpf.isHostNetworkPool = &valFalse
	}

	return obj
}
