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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	appslisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
)

// Register registers a filter
func Register(filters *filter.Filters, sm *serializer.SerializerManager) {
	filters.Register(filter.ServiceTopologyFilterName, func() (filter.Runner, error) {
		return NewFilter(sm), nil
	})
}

func NewFilter(sm *serializer.SerializerManager) *serviceTopologyFilter {
	return &serviceTopologyFilter{
		workingMode:       util.WorkingModeEdge,
		serializerManager: sm,
	}
}

type serviceTopologyFilter struct {
	serviceLister     listers.ServiceLister
	serviceSynced     cache.InformerSynced
	nodepoolLister    appslisters.NodePoolLister
	nodePoolSynced    cache.InformerSynced
	nodeGetter        filter.NodeGetter
	nodeSynced        cache.InformerSynced
	nodeName          string
	workingMode       util.WorkingMode
	serializerManager *serializer.SerializerManager
}

func (ssf *serviceTopologyFilter) Name() string {
	return filter.ServiceTopologyFilterName
}

func (ssf *serviceTopologyFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"endpoints":      sets.NewString("list", "watch"),
		"endpointslices": sets.NewString("list", "watch"),
	}
}

func (ssf *serviceTopologyFilter) SetWorkingMode(mode util.WorkingMode) error {
	ssf.workingMode = mode
	return nil
}

func (ssf *serviceTopologyFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	ssf.serviceLister = factory.Core().V1().Services().Lister()
	ssf.serviceSynced = factory.Core().V1().Services().Informer().HasSynced

	if ssf.workingMode == util.WorkingModeCloud {
		klog.Infof("prepare list/watch to sync node(%s) for cloud working mode", ssf.nodeName)
		ssf.nodeSynced = factory.Core().V1().Nodes().Informer().HasSynced
		ssf.nodeGetter = factory.Core().V1().Nodes().Lister().Get
	}

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

// TODO: should use disk storage as parameter instead of StorageWrapper
// we can internally construct a new StorageWrapper with passed-in disk storage
func (ssf *serviceTopologyFilter) SetStorageWrapper(s cachemanager.StorageWrapper) error {
	if s.Name() != disk.StorageName {
		return fmt.Errorf("serviceTopologyFilter can only support disk storage currently, cannot use %s", s.Name())
	}

	if len(ssf.nodeName) == 0 {
		return fmt.Errorf("node name for serviceTopologyFilter is not ready")
	}

	// hub agent will list/watch node from kube-apiserver when hub agent work as cloud mode
	if ssf.workingMode == util.WorkingModeCloud {
		return nil
	}
	klog.Infof("prepare local disk storage to sync node(%s) for edge working mode", ssf.nodeName)

	nodeKey, err := s.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Name:      ssf.nodeName,
		Resources: "nodes",
	})
	if err != nil {
		return fmt.Errorf("failed to get node key for %s, %v", ssf.nodeName, err)
	}
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
		key, err := s.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Name:      name,
			Resources: "nodes",
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

func (ssf *serviceTopologyFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	if ok := cache.WaitForCacheSync(stopCh, ssf.nodeSynced, ssf.serviceSynced, ssf.nodePoolSynced); !ok {
		return 0, rc, nil
	}

	handler := NewServiceTopologyFilterHandler(ssf.nodeName, ssf.serviceLister, ssf.nodepoolLister, ssf.nodeGetter)
	return filter.NewFilterReadCloser(req, ssf.serializerManager, rc, handler, ssf.Name(), stopCh)
}
