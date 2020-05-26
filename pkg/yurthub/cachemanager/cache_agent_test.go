package cachemanager

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxyutil "github.com/alibaba/openyurt/pkg/yurthub/proxy/util"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
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

	notFound := true
	for _, agent := range gotAgents {
		notFound = true
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

func TestCanCacheFor(t *testing.T) {
	s := NewFakeStorageWrapper()
	m, _ := NewCacheManager(s, nil)

	tests := []struct {
		desc        string
		userAgent   string
		verb        string
		path        string
		header      map[string]string
		expectCache bool
	}{
		{
			desc:        "no user agent",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "not default user agent",
			userAgent:   "kubelet-test",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "default user agent kubelet",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent flanneld",
			userAgent:   "flanneld",
			verb:        "POST",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent coredns",
			userAgent:   "coredns",
			verb:        "PUT",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent kube-proxy",
			userAgent:   "kube-proxy",
			verb:        "PATCH",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "default user agent edge-tunnel-agent",
			userAgent:   "edge-tunnel-agent",
			verb:        "HEAD",
			path:        "/api/v1/nodes/mynode",
			expectCache: true,
		},
		{
			desc:        "with cache header",
			userAgent:   "test1",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			header:      map[string]string{"Edge-Cache": "true"},
			expectCache: true,
		},
		{
			desc:        "with cache header false",
			userAgent:   "test2",
			verb:        "GET",
			path:        "/api/v1/nodes/mynode",
			header:      map[string]string{"Edge-Cache": "false"},
			expectCache: false,
		},
		{
			desc:        "not resource request",
			userAgent:   "test2",
			verb:        "GET",
			path:        "/healthz",
			header:      map[string]string{"Edge-Cache": "true"},
			expectCache: false,
		},
		{
			desc:        "delete request",
			userAgent:   "kubelet",
			verb:        "DELETE",
			path:        "/api/v1/nodes/mynode",
			expectCache: false,
		},
		{
			desc:        "delete collection request",
			userAgent:   "kubelet",
			verb:        "DELETE",
			path:        "/api/v1/namespaces/default/pods",
			expectCache: false,
		},
		{
			desc:        "proxy request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/proxy/namespaces/default/pods/test",
			expectCache: false,
		},
		{
			desc:        "get status sub resource request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/namespaces/default/pods/test/status",
			expectCache: true,
		},
		{
			desc:        "get not status sub resource request",
			userAgent:   "kubelet",
			verb:        "GET",
			path:        "/api/v1/namespaces/default/pods/test/proxy",
			expectCache: false,
		},
	}

	resolver := newTestRequestInfoResolver()
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {

			req, _ := http.NewRequest(tt.verb, tt.path, nil)
			if len(tt.userAgent) != 0 {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if len(tt.header) != 0 {
				for k, v := range tt.header {
					req.Header.Set(k, v)
				}
			}

			req.RemoteAddr = "127.0.0.1"

			var reqCanCache bool
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				reqCanCache = m.CanCacheFor(req)

			})

			handler = proxyutil.WithCacheHeaderCheck(handler)
			handler = proxyutil.WithRequestClientComponent(handler)
			handler = filters.WithRequestInfo(handler, resolver)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if reqCanCache != tt.expectCache {
				t.Errorf("Got request can cache %v, but expect request can cache %v", reqCanCache, tt.expectCache)
			}
		})
	}
}
