/*
Copyright 2020 The OpenYurt Authors.

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

package filter

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestUpdateFilterConfig(t *testing.T) {
	testcases := map[string]struct {
		defaultFilterCfg map[string]string
		filterCfgKey     string
		filterCfgValue   string
		result           map[string][]*requestInfo
	}{
		"update filter_cfg when profile exists": {
			filterCfgKey:   "filter_discardcloudservice",
			filterCfgValue: "w1/services#list;watch,w2/services#list",
			result: map[string][]*requestInfo{
				"discardcloudservice": {
					{
						comp:     "kube-proxy",
						resource: "services",
						verbs:    sets.NewString([]string{"list", "watch"}...),
					},
					{
						comp:     "w1",
						resource: "services",
						verbs:    sets.NewString([]string{"list", "watch"}...),
					},
					{
						comp:     "w2",
						resource: "services",
						verbs:    sets.NewString([]string{"list"}...),
					},
				},
			},
		},
		"when the configuration file is empty": {
			filterCfgKey:   "filter_endpoints",
			filterCfgValue: "",
			result: map[string][]*requestInfo{
				"endpoints": {
					{
						comp:     "nginx-ingress-controller",
						resource: "endpoints",
						verbs:    sets.NewString([]string{"list", "watch"}...),
					},
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			m := &approver{
				nameToRequests: make(map[string][]*requestInfo),
			}
			m.updateYurtHubFilterCfg(tt.filterCfgKey, tt.filterCfgValue, "")
			fileType := strings.Split(tt.filterCfgKey, "_")[1]
			reqs := m.nameToRequests[fileType]
			for _, req := range reqs {
				var flag bool
				for _, res := range tt.result[fileType] {
					if req.comp == res.comp && req.resource == res.resource && req.verbs.Equal(res.verbs) {
						flag = true
					}
				}
				if !flag {
					t.Errorf("After updating the results do not match: %v, Results to be returneds: %v", reqs, tt.result[tt.filterCfgKey])
				}
			}
		})
	}
}
