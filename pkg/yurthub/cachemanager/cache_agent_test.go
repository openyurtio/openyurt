package cachemanager

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
)

func TestInitCacheAgents(t *testing.T) {
	s := NewFakeStorageWrapper()
	m, _ := NewCacheManager(s, nil)

	// default cache agents in fake store
	b, err := s.GetRaw(cacheAgentsKey)
	if err != nil {
		t.Fatalf("failed to get agents, %v", err)
	}

	gotAgents := strings.Split(string(b), sepForAgent)
	if ok := compareAgents(gotAgents, defaultCacheAgents); !ok {
		t.Errorf("Got agents: %v, expect agents: %v", gotAgents, defaultCacheAgents)
	}

	if !compareAgents(gotAgents, m.ListCacheAgents()) {
		t.Errorf("Got agents: %v, cache agents map: %v", gotAgents, m.ListCacheAgents())
	}

	// add agents for next init cache
	_ = m.UpdateCacheAgents([]string{"agent1"})

	_, _ = NewCacheManager(s, nil)

	b2, err := s.GetRaw(cacheAgentsKey)
	if err != nil {
		t.Fatalf("failed to get agents, %v", err)
	}

	expectedAgents := append(defaultCacheAgents, "agent1")
	gotAgents2 := strings.Split(string(b2), sepForAgent)
	if ok := compareAgents(gotAgents2, expectedAgents); !ok {
		t.Errorf("Got agents: %v, expect agents: %v", gotAgents2, expectedAgents)
	}

	if !compareAgents(gotAgents2, m.ListCacheAgents()) {
		t.Errorf("Got agents: %v, cache agents map: %v", gotAgents2, m.ListCacheAgents())
	}
}

func TestUpdateCacheAgents(t *testing.T) {
	s := NewFakeStorageWrapper()
	m, _ := NewCacheManager(s, nil)

	tests := []struct {
		desc         string
		addAgents    []string
		expectAgents []string
	}{
		{desc: "add one agent", addAgents: []string{"agent1"}, expectAgents: append(defaultCacheAgents, "agent1")},
		{desc: "update with two agents", addAgents: []string{"agent2", "agent3"}, expectAgents: append(defaultCacheAgents, "agent2", "agent3")},
		{desc: "update with more two agents", addAgents: []string{"agent4", "agent5"}, expectAgents: append(defaultCacheAgents, "agent4", "agent5")},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {

			// add agents
			err := m.UpdateCacheAgents(tt.addAgents)
			if err != nil {
				t.Fatalf("failed to add cache agents, %v", err)
			}

			b, err := s.GetRaw(cacheAgentsKey)
			if err != nil {
				t.Fatalf("failed to get agents, %v", err)
			}

			gotAgents := strings.Split(string(b), sepForAgent)
			if ok := compareAgents(gotAgents, tt.expectAgents); !ok {
				t.Errorf("Got agents: %v, expect agents: %v", gotAgents, tt.expectAgents)
			}

			if !compareAgents(gotAgents, m.ListCacheAgents()) {
				t.Errorf("Got agents: %v, cache agents map: %v", gotAgents, m.ListCacheAgents())
			}
		})
	}
}

func compareAgents(gotAgents []string, expectedAgents []string) bool {
	if len(gotAgents) != len(expectedAgents) {
		return false
	}

	for _, agent := range gotAgents {
		notFound := true
		for i := range expectedAgents {
			if expectedAgents[i] == agent {
				notFound = false
				break
			}
		}

		if notFound {
			return false
		}
	}

	return true
}

func newTestRequestInfoResolver() *request.RequestInfoFactory {
	return &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
}
