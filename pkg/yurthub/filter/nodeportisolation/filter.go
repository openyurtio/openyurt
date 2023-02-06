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

package nodeportisolation

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

const (
	ServiceAnnotationNodePortListen = "nodeport.openyurt.io/listen"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.NodePortIsolationName, func() (filter.ObjectFilter, error) {
		return &nodePortIsolationFilter{}, nil
	})
}

type nodePortIsolationFilter struct {
	nodePoolName string
	workingMode  util.WorkingMode
	nodeGetter   filter.NodeGetter
	nodeSynced   cache.InformerSynced
	nodeName     string
}

func (nif *nodePortIsolationFilter) Name() string {
	return filter.NodePortIsolationName
}

func (nif *nodePortIsolationFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"services": sets.NewString("list", "watch"),
	}
}

func (nif *nodePortIsolationFilter) SetNodePoolName(name string) error {
	nif.nodePoolName = name
	return nil
}

func (nif *nodePortIsolationFilter) SetNodeName(nodeName string) error {
	nif.nodeName = nodeName
	return nil
}

func (nif *nodePortIsolationFilter) SetWorkingMode(mode util.WorkingMode) error {
	nif.workingMode = mode
	return nil
}

func (nif *nodePortIsolationFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	if nif.workingMode == util.WorkingModeCloud {
		klog.Infof("prepare list/watch to sync node(%s) for cloud working mode", nif.nodeName)
		nif.nodeSynced = factory.Core().V1().Nodes().Informer().HasSynced
		nif.nodeGetter = factory.Core().V1().Nodes().Lister().Get
	}

	return nil
}

func (nif *nodePortIsolationFilter) SetStorageWrapper(s cachemanager.StorageWrapper) error {
	if s.Name() != disk.StorageName {
		return fmt.Errorf("nodePortIsolationFilter can only support disk storage currently, cannot use %s", s.Name())
	}

	if len(nif.nodeName) == 0 {
		return fmt.Errorf("node name for nodePortIsolationFilter is not set")
	}

	// hub agent will list/watch node from kube-apiserver when hub agent work as cloud mode
	if nif.workingMode == util.WorkingModeCloud {
		return nil
	}
	klog.Infof("prepare local disk storage to sync node(%s) for edge working mode", nif.nodeName)

	nodeKey, err := s.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Name:      nif.nodeName,
		Resources: "nodes",
		Group:     "",
		Version:   "v1",
	})
	if err != nil {
		return fmt.Errorf("failed to get node key for %s, %v", nif.nodeName, err)
	}
	nif.nodeSynced = func() bool {
		obj, err := s.Get(nodeKey)
		if err != nil || obj == nil {
			return false
		}

		if _, ok := obj.(*v1.Node); !ok {
			return false
		}

		return true
	}

	nif.nodeGetter = func(name string) (*v1.Node, error) {
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

func (nif *nodePortIsolationFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	if ok := cache.WaitForCacheSync(stopCh, nif.nodeSynced); !ok {
		return obj
	}

	switch v := obj.(type) {
	case *v1.ServiceList:
		var svcNew []v1.Service
		for i := range v.Items {
			svc := nif.isolateNodePortService(&v.Items[i])
			if svc != nil {
				svcNew = append(svcNew, *svc)
			}
		}
		v.Items = svcNew
		return v
	case *v1.Service:
		return nif.isolateNodePortService(v)
	default:
		return v
	}
}

func (nif *nodePortIsolationFilter) isolateNodePortService(svc *v1.Service) *v1.Service {
	nodePoolName := nif.nodePoolName
	if len(nodePoolName) == 0 {
		node, err := nif.nodeGetter(nif.nodeName)
		if err != nil {
			klog.Warningf("skip isolateNodePortService filter, failed to get node(%s), %v", nif.nodeName, err)
			return svc
		}
		nodePoolName = node.Labels[nodepoolv1alpha1.LabelCurrentNodePool]
	}

	// node is not located in NodePool, keep the NodePort service the same as native K8s
	if len(nodePoolName) == 0 {
		return svc
	}

	nsName := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	if svc.Spec.Type == v1.ServiceTypeNodePort || svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		if _, ok := svc.Annotations[ServiceAnnotationNodePortListen]; ok {
			nodePoolConf := getNodePoolConfiguration(svc.Annotations[ServiceAnnotationNodePortListen])
			if nodePoolConf.Len() != 0 && isNodePoolEnabled(nodePoolConf, nodePoolName) {
				return svc
			} else {
				klog.V(2).Infof("nodePort service(%s) is disabled in nodePool(%s) by nodePortIsolationFilter", nsName, nodePoolName)
				return nil
			}
		}
	}

	return svc
}

func getNodePoolConfiguration(v string) sets.String {
	nodePoolConf := sets.NewString()
	nodePoolsForValidation := sets.NewString()
	for _, name := range strings.Split(v, ",") {
		name = strings.TrimSpace(name)
		trimmedName := strings.TrimPrefix(name, "-")
		if len(trimmedName) != 0 && !nodePoolsForValidation.Has(trimmedName) {
			nodePoolsForValidation.Insert(trimmedName)
			nodePoolConf.Insert(name)
		}
	}

	return nodePoolConf
}

func isNodePoolEnabled(nodePoolConf sets.String, name string) bool {
	if nodePoolConf.Has(name) {
		return true
	}

	if nodePoolConf.Has(fmt.Sprintf("-%s", name)) {
		return false
	}

	if nodePoolConf.Has("*") {
		return true
	}

	// the nodepool has not been configured
	return false
}
