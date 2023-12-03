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
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	ServiceAnnotationNodePortListen = "nodeport.openyurt.io/listen"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.NodePortIsolationFilterName, func() (filter.ObjectFilter, error) {
		return NewNodePortIsolationFilter()
	})
}

type nodePortIsolationFilter struct {
	nodePoolName string
	nodeName     string
	client       kubernetes.Interface
}

func NewNodePortIsolationFilter() (filter.ObjectFilter, error) {
	return &nodePortIsolationFilter{}, nil
}

func (nif *nodePortIsolationFilter) Name() string {
	return filter.NodePortIsolationFilterName
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

func (nif *nodePortIsolationFilter) SetKubeClient(client kubernetes.Interface) error {
	nif.client = client
	return nil
}

func (nif *nodePortIsolationFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
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
	nodePoolName := nif.resolveNodePoolName()
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
				klog.V(2).Infof("service(%s) is disabled in nodePool(%s) by nodePortIsolationFilter", nsName, nodePoolName)
				return nil
			}
		}
	}

	return svc
}

func (nif *nodePortIsolationFilter) resolveNodePoolName() string {
	if len(nif.nodePoolName) != 0 {
		return nif.nodePoolName
	}

	node, err := nif.client.CoreV1().Nodes().Get(context.Background(), nif.nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("skip isolateNodePortService filter, could not get node(%s), %v", nif.nodeName, err)
		return nif.nodePoolName
	}
	nif.nodePoolName = node.Labels[projectinfo.GetNodePoolLabel()]
	return nif.nodePoolName
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
