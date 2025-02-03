/*
Copyright 2022 The OpenYurt Authors.

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

package manager

import (
	"net/http"
	"strconv"

	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	yurtoptions "github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/approver"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/objectfilter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/responsefilter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type Manager struct {
	filter.Approver
	nameToObjectFilter map[string]filter.ObjectFilter
	serializerManager  *serializer.SerializerManager
	resourceSyncers    []filter.ResourceSyncer
}

func NewFilterManager(options *yurtoptions.YurtHubOptions,
	sharedFactory informers.SharedInformerFactory,
	dynamicSharedFactory dynamicinformer.DynamicSharedInformerFactory,
	proxiedClient kubernetes.Interface,
	serializerManager *serializer.SerializerManager,
	configManager *configuration.Manager) (filter.FilterFinder, error) {
	var err error
	nameToFilters := make(map[string]filter.ObjectFilter)
	if options.EnableResourceFilter {
		// 1. new base filters
		if options.WorkingMode == string(util.WorkingModeCloud) {
			options.DisabledResourceFilters = append(options.DisabledResourceFilters, yurtoptions.DisabledInCloudMode...)
		}
		filters := base.NewFilters(options.DisabledResourceFilters)

		// 2. register all filter factory
		yurtoptions.RegisterAllFilters(filters)

		// 3. prepare filter initializer chain
		mutatedMasterServicePort := strconv.Itoa(options.YurtHubProxySecurePort)
		mutatedMasterServiceHost := options.YurtHubProxyHost
		if options.EnableDummyIf {
			mutatedMasterServiceHost = options.HubAgentDummyIfIP
		}
		genericInitializer := initializer.New(sharedFactory, proxiedClient, options.NodeName, options.NodePoolName, mutatedMasterServiceHost, mutatedMasterServicePort)
		nodesInitializer := initializer.NewNodesInitializer(options.EnableNodePool, options.EnablePoolServiceTopology, dynamicSharedFactory)
		initializerChain := base.Initializers{}
		initializerChain = append(initializerChain, genericInitializer, nodesInitializer)

		// 4. initialize all object filters
		nameToFilters, err = filters.NewFromFilters(initializerChain)
		if err != nil {
			return nil, err
		}
	}

	resourceSyncers := make([]filter.ResourceSyncer, 0)
	for name, objFilter := range nameToFilters {
		if resourceSyncer, ok := objFilter.(filter.ResourceSyncer); ok {
			klog.Infof("filter %s need to sync resource before starting to work", name)
			resourceSyncers = append(resourceSyncers, resourceSyncer)
		}
	}

	// 5. new filter manager including approver and nameToObjectFilter
	// if resource filters are disabled, nameToObjectFilter and resourceSyncers will be empty silces.
	return &Manager{
		Approver:           approver.NewApprover(options.NodeName, configManager),
		nameToObjectFilter: nameToFilters,
		serializerManager:  serializerManager,
		resourceSyncers:    resourceSyncers,
	}, nil
}

func (m *Manager) HasSynced() bool {
	for i := range m.resourceSyncers {
		if !m.resourceSyncers[i].HasSynced() {
			return false
		}
	}
	return true
}

func (m *Manager) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
	if len(m.nameToObjectFilter) == 0 {
		return nil, false
	}

	approved, filterNames := m.Approver.Approve(req)
	if approved {
		objectFilters := make([]filter.ObjectFilter, 0)
		for i := range filterNames {
			if objectFilter, ok := m.nameToObjectFilter[filterNames[i]]; ok {
				objectFilters = append(objectFilters, objectFilter)
			}
		}

		if len(objectFilters) == 0 {
			return nil, false
		}

		return responsefilter.CreateResponseFilter(objectFilters, m.serializerManager), true
	}

	return nil, false
}

func (m *Manager) FindObjectFilter(req *http.Request) (filter.ObjectFilter, bool) {
	if len(m.nameToObjectFilter) == 0 {
		return nil, false
	}

	approved, filterNames := m.Approver.Approve(req)
	if !approved {
		return nil, false
	}

	objectFilters := make([]filter.ObjectFilter, 0)
	for i := range filterNames {
		if objectFilter, ok := m.nameToObjectFilter[filterNames[i]]; ok {
			objectFilters = append(objectFilters, objectFilter)
		}
	}

	if len(objectFilters) == 0 {
		return nil, false
	}

	return objectfilter.CreateFilterChain(objectFilters), true
}
