/*
Copyright 2024 The OpenYurt Authors.

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

package options

import (
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/forwardkubesvctraffic"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/inclusterconfig"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/serviceenvupdater"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

var (
	// DisabledInCloudMode contains the filters that should be disabled when yurthub is working in cloud mode.
	DisabledInCloudMode = []string{discardcloudservice.FilterName, forwardkubesvctraffic.FilterName, serviceenvupdater.FilterName}

	// FilterToComponentsResourcesAndVerbs is used to specify which request with resource and verb from component is supported by the filter.
	// When adding a new filter, It is essential to update the FilterToComponentsResourcesAndVerbs map
	// to include this new filter along with the component, resource and request verbs it supports.
	FilterToComponentsResourcesAndVerbs = map[string]struct {
		DefaultComponents []string
		ResourceAndVerbs  map[string][]string
	}{
		masterservice.FilterName: {
			DefaultComponents: []string{"kubelet"},
			ResourceAndVerbs: map[string][]string{
				"services": {"list", "watch"},
			},
		},
		discardcloudservice.FilterName: {
			DefaultComponents: []string{"kube-proxy"},
			ResourceAndVerbs: map[string][]string{
				"services": {"list", "watch"},
			},
		},
		servicetopology.FilterName: {
			DefaultComponents: []string{"kube-proxy", "coredns", "nginx-ingress-controller"},
			ResourceAndVerbs: map[string][]string{
				"endpoints":      {"list", "watch"},
				"endpointslices": {"list", "watch"},
			},
		},
		inclusterconfig.FilterName: {
			DefaultComponents: []string{"kubelet"},
			ResourceAndVerbs: map[string][]string{
				"configmaps": {"get", "list", "watch"},
			},
		},
		nodeportisolation.FilterName: {
			DefaultComponents: []string{"kube-proxy"},
			ResourceAndVerbs: map[string][]string{
				"services": {"list", "watch"},
			},
		},
		forwardkubesvctraffic.FilterName: {
			DefaultComponents: []string{"kube-proxy"},
			ResourceAndVerbs: map[string][]string{
				"endpointslices": {"list", "watch"},
			},
		},
		serviceenvupdater.FilterName: {
			DefaultComponents: []string{"kubelet"},
			ResourceAndVerbs: map[string][]string{
				"pods": {"list", "watch", "get", "patch"},
			},
		},
	}
)

// RegisterAllFilters by order, the front registered filter will be
// called before the latter registered ones.
// Attention:
// when you add a new filter, you should register new filter here.
func RegisterAllFilters(filters *base.Filters) {
	servicetopology.Register(filters)
	masterservice.Register(filters)
	discardcloudservice.Register(filters)
	inclusterconfig.Register(filters)
	nodeportisolation.Register(filters)
	forwardkubesvctraffic.Register(filters)
	serviceenvupdater.Register(filters)
}
