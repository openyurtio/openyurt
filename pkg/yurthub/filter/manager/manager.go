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

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/inclusterconfig"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

type Manager struct {
	filter.Approver
	NameToFilterRunner map[string]filter.Runner
}

func NewFilterManager(options *options.YurtHubOptions,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	serializerManager *serializer.SerializerManager,
	storageWrapper cachemanager.StorageWrapper,
	apiserverAddr string) (*Manager, error) {
	if !options.EnableResourceFilter {
		return nil, nil
	}

	if options.WorkingMode == string(util.WorkingModeCloud) {
		options.DisabledResourceFilters = append(options.DisabledResourceFilters, filter.DisabledInCloudMode...)
	}
	filters := filter.NewFilters(options.DisabledResourceFilters)
	registerAllFilters(filters, serializerManager)

	mutatedMasterServiceHost, mutatedMasterServicePort, _ := net.SplitHostPort(apiserverAddr)
	if options.AccessServerThroughHub {
		mutatedMasterServicePort = strconv.Itoa(options.YurtHubProxySecurePort)
		if options.EnableDummyIf {
			mutatedMasterServiceHost = options.HubAgentDummyIfIP
		} else {
			mutatedMasterServiceHost = options.YurtHubProxyHost
		}
	}

	runners, err := createFilterRunners(filters, sharedFactory, yurtSharedFactory, storageWrapper, util.WorkingMode(options.WorkingMode), options.NodeName, mutatedMasterServiceHost, mutatedMasterServicePort)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		NameToFilterRunner: make(map[string]filter.Runner),
	}

	filterSupportedResAndVerbs := make(map[string]map[string]sets.String)
	for i := range runners {
		m.NameToFilterRunner[runners[i].Name()] = runners[i]
		filterSupportedResAndVerbs[runners[i].Name()] = runners[i].SupportedResourceAndVerbs()
	}
	m.Approver = filter.NewApprover(sharedFactory, filterSupportedResAndVerbs)

	return m, nil
}

func (m *Manager) FindRunner(req *http.Request) (bool, filter.Runner) {
	approved, runnerName := m.Approver.Approve(req)
	if approved {
		if runner, ok := m.NameToFilterRunner[runnerName]; ok {
			return true, runner
		}
	}

	return false, nil
}

// createFilterRunners return a slice of filter runner that initializations completed.
func createFilterRunners(filters *filter.Filters,
	sharedFactory informers.SharedInformerFactory,
	yurtSharedFactory yurtinformers.SharedInformerFactory,
	storageWrapper cachemanager.StorageWrapper,
	workingMode util.WorkingMode,
	nodeName, mutatedMasterServiceHost, mutatedMasterServicePort string) ([]filter.Runner, error) {
	if filters == nil {
		return nil, nil
	}

	genericInitializer := initializer.New(sharedFactory, yurtSharedFactory, storageWrapper, nodeName, mutatedMasterServiceHost, mutatedMasterServicePort, workingMode)
	initializerChain := filter.FilterInitializers{}
	initializerChain = append(initializerChain, genericInitializer)
	return filters.NewFromFilters(initializerChain)
}

// registerAllFilters by order, the front registered filter will be
// called before the latter registered ones.
func registerAllFilters(filters *filter.Filters, sm *serializer.SerializerManager) {
	servicetopology.Register(filters, sm)
	masterservice.Register(filters, sm)
	discardcloudservice.Register(filters, sm)
	inclusterconfig.Register(filters, sm)
}
