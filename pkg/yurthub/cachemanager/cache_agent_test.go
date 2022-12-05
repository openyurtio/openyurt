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

package cachemanager

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func TestUpdateCacheAgents(t *testing.T) {
	testcases := map[string]struct {
		desc          string
		initAgents    []string
		cacheAgents   string
		resultAgents  sets.String
		deletedAgents sets.String
	}{
		"two new agents updated": {
			initAgents:    []string{},
			cacheAgents:   "agent1,agent2",
			resultAgents:  sets.NewString(append([]string{"agent1", "agent2"}, util.DefaultCacheAgents...)...),
			deletedAgents: sets.String{},
		},
		"two new agents updated but an old agent deleted": {
			initAgents:    []string{"agent1", "agent2"},
			cacheAgents:   "agent2,agent3",
			resultAgents:  sets.NewString(append([]string{"agent2", "agent3"}, util.DefaultCacheAgents...)...),
			deletedAgents: sets.NewString("agent1"),
		},
		"no agents updated ": {
			initAgents:    []string{"agent1", "agent2"},
			cacheAgents:   "agent1,agent2",
			resultAgents:  sets.NewString(append([]string{"agent1", "agent2"}, util.DefaultCacheAgents...)...),
			deletedAgents: sets.String{},
		},
		"no agents updated with default": {
			initAgents:    []string{"agent1", "agent2", "kubelet"},
			cacheAgents:   "agent1,agent2",
			resultAgents:  sets.NewString(append([]string{"agent1", "agent2"}, util.DefaultCacheAgents...)...),
			deletedAgents: sets.String{},
		},
		"empty agents added ": {
			initAgents:    []string{},
			cacheAgents:   "",
			resultAgents:  sets.NewString(util.DefaultCacheAgents...),
			deletedAgents: sets.String{},
		},
	}
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			m := &CacheAgent{
				agents: sets.NewString(tt.initAgents...),
			}

			m.updateCacheAgents(strings.Join(tt.initAgents, ","), "")

			// add agents
			deletedAgents := m.updateCacheAgents(tt.cacheAgents, "")

			if !deletedAgents.Equal(tt.deletedAgents) {
				t.Errorf("Got deleted agents: %v, expect agents: %v", deletedAgents, tt.deletedAgents)
			}

			if !m.agents.Equal(tt.resultAgents) {
				t.Errorf("Got cache agents: %v, expect agents: %v", m.agents, tt.resultAgents)
			}
		})
	}
}
