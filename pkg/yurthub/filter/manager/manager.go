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
	"sync"

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
	mu                 sync.RWMutex
	nameToObjectFilter map[string]filter.ObjectFilter
	serializerManager  *serializer.SerializerManager
	resourceSyncers    []filter.ResourceSyncer

	// dependencies for dynamic filter rebuilding
	options              *yurtoptions.YurtHubOptions
	sharedFactory        informers.SharedInformerFactory
	dynamicSharedFactory dynamicinformer.DynamicSharedInformerFactory
	client               kubernetes.Interface
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
	m := &Manager{
		Approver:             approver.NewApprover(options.NodeName, configManager),
		nameToObjectFilter:   nameToFilters,
		serializerManager:    serializerManager,
		resourceSyncers:      resourceSyncers,
		options:              options,
		sharedFactory:        sharedFactory,
		dynamicSharedFactory: dynamicSharedFactory,
		client:               proxiedClient,
	}

	return m, nil
}

func (m *Manager) HasSynced() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.resourceSyncers {
		if !m.resourceSyncers[i].HasSynced() {
			return false
		}
	}
	return true
}

func (m *Manager) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.nameToObjectFilter) == 0 {
		return nil, false
	}

	approved, filterNames := m.Approve(req)
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.nameToObjectFilter) == 0 {
		return nil, false
	}

	approved, filterNames := m.Approve(req)
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

// Reset rebuilds the filter manager's internal state (nameToObjectFilter and resourceSyncers)
// from the new configuration data. It must be called under the write lock (mu).
// The method builds new maps into local variables first, then atomically swaps them so that
// a failed Reset never leaves FilterManager in an inconsistent state serving live traffic with
// stale filters. If construction fails, the error is returned and the previous state is left intact.
//
// The caller must ensure that no goroutine is concurrently calling FindResponseFilter or
// FindObjectFilter while Reset is running, or external synchronization must be provided.
func (m *Manager) Reset(cmData map[string]string) error {

	// Step 1: build new state in local variables first (never mutate struct fields until success)
	newNameToFilters := make(map[string]filter.ObjectFilter)
	newResourceSyncers := make([]filter.ResourceSyncer, 0)

	if m.options != nil && m.options.EnableResourceFilter {
		// Re-create the filter registry with the current disabled list.
		filtersReg := base.NewFilters(m.options.DisabledResourceFilters)
		// Re-register all filter factories.
		yurtoptions.RegisterAllFilters(filtersReg)

		// Re-build the initializer chain using the new config.
		mutatedMasterServicePort := strconv.Itoa(m.options.YurtHubProxySecurePort)
		mutatedMasterServiceHost := m.options.YurtHubProxyHost
		if m.options.EnableDummyIf {
			mutatedMasterServiceHost = m.options.HubAgentDummyIfIP
		}
		genericInitializer := initializer.New(m.sharedFactory, m.client, m.options.NodeName, m.options.NodePoolName,
			mutatedMasterServiceHost, mutatedMasterServicePort)
		nodesInitializer := initializer.NewNodesInitializer(m.options.EnableNodePool, m.options.EnablePoolServiceTopology, m.dynamicSharedFactory)
		initializerChain := base.Initializers{}
		initializerChain = append(initializerChain, genericInitializer, nodesInitializer)

		// Initialize all object filters with the new chain.
		var err error
		newNameToFilters, err = filtersReg.NewFromFilters(initializerChain)
		if err != nil {
			klog.Errorf("could not rebuild filters during Reset, %v", err)
			return err
		}

		// Collect resource syncers from the newly initialized filters.
		for name, objFilter := range newNameToFilters {
			if resourceSyncer, ok := objFilter.(filter.ResourceSyncer); ok {
				klog.Infof("filter %s need to sync resource before starting to work (reset)", name)
				newResourceSyncers = append(newResourceSyncers, resourceSyncer)
			}
		}
	}

	// Step 2: only assign the new maps after successful construction.
	// This ensures a failed Reset never leaves FilterManager half-updated.
	m.mu.Lock()
	m.nameToObjectFilter = newNameToFilters
	m.resourceSyncers = newResourceSyncers
	m.mu.Unlock()

	klog.Infof("FilterManager state rebuilt successfully after ConfigMap update: %d object filters, %d resource syncers",
		len(m.nameToObjectFilter), len(m.resourceSyncers))
	return nil
}
