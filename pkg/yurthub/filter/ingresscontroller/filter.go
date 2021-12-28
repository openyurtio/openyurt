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

package ingresscontroller

import (
	"fmt"
	"io"
	"net/http"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	filterutil "github.com/openyurtio/openyurt/pkg/yurthub/filter/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	appslisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.IngressControllerFilterName, func() (filter.Interface, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *ingressControllerFilter {
	return &ingressControllerFilter{
		Approver:    filter.NewApprover("nginx-ingress-controller", "endpoints", []string{"list", "watch"}...),
		workingMode: util.WorkingModeEdge,
		stopCh:      make(chan struct{}),
	}
}

type ingressControllerFilter struct {
	*filter.Approver
	serviceLister     listers.ServiceLister
	serviceSynced     cache.InformerSynced
	nodepoolLister    appslisters.NodePoolLister
	nodePoolSynced    cache.InformerSynced
	nodeGetter        filter.NodeGetter
	nodeSynced        cache.InformerSynced
	nodeName          string
	serializerManager *serializer.SerializerManager
	workingMode       util.WorkingMode
	stopCh            chan struct{}
}

func (ssf *ingressControllerFilter) SetWorkingMode(mode util.WorkingMode) error {
	ssf.workingMode = mode
	return nil
}

func (ssf *ingressControllerFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	ssf.serviceLister = factory.Core().V1().Services().Lister()
	ssf.serviceSynced = factory.Core().V1().Services().Informer().HasSynced

	if ssf.workingMode == util.WorkingModeCloud {
		klog.Infof("prepare list/watch to sync node(%s) for cloud working mode", ssf.nodeName)
		ssf.nodeSynced = factory.Core().V1().Nodes().Informer().HasSynced
		ssf.nodeGetter = factory.Core().V1().Nodes().Lister().Get
	}

	return nil
}

func (ssf *ingressControllerFilter) SetYurtSharedInformerFactory(yurtFactory yurtinformers.SharedInformerFactory) error {
	ssf.nodepoolLister = yurtFactory.Apps().V1alpha1().NodePools().Lister()
	ssf.nodePoolSynced = yurtFactory.Apps().V1alpha1().NodePools().Informer().HasSynced

	return nil
}

func (ssf *ingressControllerFilter) SetNodeName(nodeName string) error {
	ssf.nodeName = nodeName

	return nil
}

func (ssf *ingressControllerFilter) SetStorageWrapper(s cachemanager.StorageWrapper) error {
	if len(ssf.nodeName) == 0 {
		return fmt.Errorf("node name for ingressControllerFilter is not ready")
	}

	// hub agent will list/watch node from kube-apiserver when hub agent work as cloud mode
	if ssf.workingMode == util.WorkingModeCloud {
		return nil
	}
	klog.Infof("prepare local disk storage to sync node(%s) for edge working mode", ssf.nodeName)

	nodeKey := fmt.Sprintf("kubelet/nodes/%s", ssf.nodeName)
	ssf.nodeSynced = func() bool {
		obj, err := s.Get(nodeKey)
		if err != nil || obj == nil {
			return false
		}

		if _, ok := obj.(*v1.Node); !ok {
			return false
		}

		return true
	}

	ssf.nodeGetter = func(name string) (*v1.Node, error) {
		obj, err := s.Get(nodeKey)
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

func (ssf *ingressControllerFilter) SetSerializerManager(s *serializer.SerializerManager) error {
	ssf.serializerManager = s
	return nil
}

func (ssf *ingressControllerFilter) Approve(comp, resource, verb string) bool {
	if !ssf.Approver.Approve(comp, resource, verb) {
		return false
	}

	if ok := cache.WaitForCacheSync(ssf.stopCh, ssf.nodeSynced, ssf.serviceSynced, ssf.nodePoolSynced); !ok {
		return false
	}

	return true
}

func (ssf *ingressControllerFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	s := filterutil.CreateSerializer(req, ssf.serializerManager)
	if s == nil {
		klog.Errorf("skip filter, failed to create serializer in ingressControllerFilter")
		return 0, rc, nil
	}

	handler := NewIngressControllerFilterHandler(ssf.nodeName, s, ssf.serviceLister, ssf.nodepoolLister, ssf.nodeGetter)
	return filter.NewFilterReadCloser(req, rc, handler, s, filter.IngressControllerFilterName, stopCh)
}
