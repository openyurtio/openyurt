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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	yurtoptions "github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/approver"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/responsefilter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type Manager struct {
	filter.Approver
	nameToObjectFilter map[string]filter.ObjectFilter
	serializerManager  *serializer.SerializerManager
}

func NewFilterManager(options *yurtoptions.YurtHubOptions,
	sharedFactory informers.SharedInformerFactory,
	dynamicSharedFactory dynamicinformer.DynamicSharedInformerFactory,
	proxiedClient kubernetes.Interface,
	serializerManager *serializer.SerializerManager) (*Manager, error) {
	if !options.EnableResourceFilter {
		return nil, nil
	}

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
	objFilters, err := filters.NewFromFilters(initializerChain)
	if err != nil {
		return nil, err
	}

	// 5. new filter manager including approver and nameToObjectFilter
	m := &Manager{
		nameToObjectFilter: make(map[string]filter.ObjectFilter),
		serializerManager:  serializerManager,
	}

	filterSupportedResAndVerbs := make(map[string]map[string]sets.String)
	for i := range objFilters {
		m.nameToObjectFilter[objFilters[i].Name()] = objFilters[i]
		filterSupportedResAndVerbs[objFilters[i].Name()] = objFilters[i].SupportedResourceAndVerbs()
	}
	m.Approver = approver.NewApprover(sharedFactory, filterSupportedResAndVerbs)

	return m, nil
}

func (m *Manager) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
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
