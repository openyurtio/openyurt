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
	"net"
	"net/http"
	"strconv"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/inclusterconfig"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

type Manager struct {
	filter.Approver
	nameToObjectFilter map[string]filter.ObjectFilter
	serializerManager  *serializer.SerializerManager
}

func NewFilterManager(options *options.YurtHubOptions,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	proxiedClient kubernetes.Interface,
	serializerManager *serializer.SerializerManager,
	apiserverAddr string) (*Manager, error) {
	if !options.EnableResourceFilter {
		return nil, nil
	}

	if options.WorkingMode == string(util.WorkingModeCloud) {
		options.DisabledResourceFilters = append(options.DisabledResourceFilters, filter.DisabledInCloudMode...)
	}
	filters := filter.NewFilters(options.DisabledResourceFilters)
	registerAllFilters(filters)

	mutatedMasterServiceHost, mutatedMasterServicePort, _ := net.SplitHostPort(apiserverAddr)
	if options.AccessServerThroughHub {
		mutatedMasterServicePort = strconv.Itoa(options.YurtHubProxySecurePort)
		if options.EnableDummyIf {
			mutatedMasterServiceHost = options.HubAgentDummyIfIP
		} else {
			mutatedMasterServiceHost = options.YurtHubProxyHost
		}
	}

	objFilters, err := createObjectFilters(filters, sharedFactory, yurtSharedFactory, proxiedClient, options.NodeName, options.NodePoolName, mutatedMasterServiceHost, mutatedMasterServicePort)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		nameToObjectFilter: make(map[string]filter.ObjectFilter),
		serializerManager:  serializerManager,
	}

	filterSupportedResAndVerbs := make(map[string]map[string]sets.String)
	for i := range objFilters {
		m.nameToObjectFilter[objFilters[i].Name()] = objFilters[i]
		filterSupportedResAndVerbs[objFilters[i].Name()] = objFilters[i].SupportedResourceAndVerbs()
	}
	m.Approver = filter.NewApprover(sharedFactory, filterSupportedResAndVerbs)

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

		return filter.CreateResponseFilter(filter.CreateFilterChain(objectFilters), m.serializerManager), true
	}

	return nil, false
}

// createObjectFilters return all object filters that initializations completed.
func createObjectFilters(filters *filter.Filters,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	proxiedClient kubernetes.Interface,
	nodeName, nodePoolName, mutatedMasterServiceHost, mutatedMasterServicePort string) ([]filter.ObjectFilter, error) {
	if filters == nil {
		return nil, nil
	}

	genericInitializer := initializer.New(sharedFactory, yurtSharedFactory, proxiedClient, nodeName, nodePoolName, mutatedMasterServiceHost, mutatedMasterServicePort)
	initializerChain := filter.Initializers{}
	initializerChain = append(initializerChain, genericInitializer)
	return filters.NewFromFilters(initializerChain)
}

// registerAllFilters by order, the front registered filter will be
// called before the latter registered ones.
func registerAllFilters(filters *filter.Filters) {
	servicetopology.Register(filters)
	masterservice.Register(filters)
	discardcloudservice.Register(filters)
	inclusterconfig.Register(filters)
	nodeportisolation.Register(filters)
}
