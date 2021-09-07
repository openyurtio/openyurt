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
	"io"
	"net/http"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	filterutil "github.com/openyurtio/openyurt/pkg/yurthub/filter/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	appslisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.ServiceTopologyFilterName, func() (filter.Interface, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *serviceTopologyFilter {
	return &serviceTopologyFilter{
		Approver: filter.NewApprover("kube-proxy", "endpointslices", []string{"list", "watch"}...),
		stopCh:   make(chan struct{}),
	}
}

type serviceTopologyFilter struct {
	*filter.Approver
	serviceLister     listers.ServiceLister
	serviceSynced     cache.InformerSynced
	nodepoolLister    appslisters.NodePoolLister
	nodePoolSynced    cache.InformerSynced
	nodeGetter        filter.NodeGetter
	nodeSynced        cache.InformerSynced
	nodeName          string
	serializerManager *serializer.SerializerManager
	stopCh            chan struct{}
}

func (ssf *serviceTopologyFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	ssf.serviceLister = factory.Core().V1().Services().Lister()
	ssf.serviceSynced = factory.Core().V1().Services().Informer().HasSynced

	return nil
}

func (ssf *serviceTopologyFilter) SetYurtSharedInformerFactory(yurtFactory yurtinformers.SharedInformerFactory) error {
	ssf.nodepoolLister = yurtFactory.Apps().V1alpha1().NodePools().Lister()
	ssf.nodePoolSynced = yurtFactory.Apps().V1alpha1().NodePools().Informer().HasSynced

	return nil
}

func (ssf *serviceTopologyFilter) SetNodeName(nodeName string) error {
	ssf.nodeName = nodeName

	return nil
}

func (ssf *serviceTopologyFilter) SetStorageWrapper(s cachemanager.StorageWrapper) error {
	if len(ssf.nodeName) == 0 {
		return fmt.Errorf("node name for serviceTopologyFilter is not ready")
	}

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

	ssf.nodeGetter = func() (*v1.Node, error) {
		obj, err := s.Get(nodeKey)
		if err != nil {
			return nil, err
		} else if obj == nil {
			return nil, fmt.Errorf("node(%s) is not ready", ssf.nodeName)
		}

		if node, ok := obj.(*v1.Node); ok {
			return node, nil
		}

		return nil, fmt.Errorf("node(%s) is not found", ssf.nodeName)
	}

	return nil
}

func (ssf *serviceTopologyFilter) SetSerializerManager(s *serializer.SerializerManager) error {
	ssf.serializerManager = s
	return nil
}

func (ssf *serviceTopologyFilter) Approve(comp, resource, verb string) bool {
	if !ssf.Approver.Approve(comp, resource, verb) {
		return false
	}

	if ok := cache.WaitForCacheSync(ssf.stopCh, ssf.nodeSynced, ssf.serviceSynced, ssf.nodePoolSynced); !ok {
		return false
	}

	return true
}

func (ssf *serviceTopologyFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	s := filterutil.CreateSerializer(req, ssf.serializerManager)
	if s == nil {
		klog.Errorf("skip filter, failed to create serializer in serviceTopologyFilter")
		return 0, rc, nil
	}

	handler := NewServiceTopologyFilterHandler(ssf.nodeName, s, ssf.serviceLister, ssf.nodepoolLister, ssf.nodeGetter)
	return filter.NewFilterReadCloser(req, rc, handler, s, filter.ServiceTopologyFilterName, stopCh)
}
