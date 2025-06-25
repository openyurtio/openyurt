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

package approver

import (
	"net/http"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/configuration"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type approver struct {
	skipRequestUserAgentList sets.Set[string]
	configManager            *configuration.Manager
}

func NewApprover(nodeName string, configManager *configuration.Manager) filter.Approver {
	na := &approver{
		skipRequestUserAgentList: sets.New[string](projectinfo.GetHubName(), util.MultiplexerProxyClientUserAgentPrefix+nodeName),
		configManager:            configManager,
	}
	return na
}

func (a *approver) Approve(req *http.Request) (bool, []string) {
	if comp, ok := util.TruncatedClientComponentFrom(req.Context()); !ok {
		return false, []string{}
	} else {
		if a.skipRequestUserAgentList.Has(comp) {
			return false, []string{}
		}
	}

	filterNames := a.configManager.FindFiltersFor(req)
	if len(filterNames) == 0 {
		return false, filterNames
	}

	return true, filterNames
}
