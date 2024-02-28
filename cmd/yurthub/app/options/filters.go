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
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

var (
	// DisabledInCloudMode contains the filters that should be disabled when yurthub is working in cloud mode.
	DisabledInCloudMode = []string{discardcloudservice.FilterName, forwardkubesvctraffic.FilterName}

	// SupportedComponentsForFilter is used for specifying which components are supported by filters as default setting.
	SupportedComponentsForFilter = map[string]string{
		masterservice.FilterName:         "kubelet",
		discardcloudservice.FilterName:   "kube-proxy",
		servicetopology.FilterName:       "kube-proxy, coredns, nginx-ingress-controller",
		inclusterconfig.FilterName:       "kubelet",
		nodeportisolation.FilterName:     "kube-proxy",
		forwardkubesvctraffic.FilterName: "kube-proxy",
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
}
