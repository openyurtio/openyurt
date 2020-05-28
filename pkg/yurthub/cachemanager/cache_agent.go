package cachemanager

import (
	"strings"

	"k8s.io/klog"
)

var (
	defaultCacheAgents = []string{
		"kubelet",
		"kube-proxy",
		"flanneld",
		"coredns",
		"edge-tunnel-agent",
	}
	cacheAgentsKey = "_internal/cache-manager/cache-agent.conf"
	sepForAgent    = ","
)

func (cm *cacheManager) initCacheAgents() error {
	agents := make([]string, 0)
	b, err := cm.storage.GetRaw(cacheAgentsKey)
	if err == nil && len(b) != 0 {
		localAgents := strings.Split(string(b), sepForAgent)
		if len(localAgents) < len(defaultCacheAgents) {
			err = cm.storage.Delete(cacheAgentsKey)
			if err != nil {
				klog.Errorf("failed to delete agents cache, %v", err)
				return err
			}
		} else {
			agents = append(agents, localAgents...)
			for _, agent := range localAgents {
				cm.cacheAgents[agent] = false
			}
		}
	}
	for _, agent := range defaultCacheAgents {
		if cm.cacheAgents == nil {
			cm.cacheAgents = make(map[string]bool)
		}

		if _, ok := cm.cacheAgents[agent]; !ok {
			agents = append(agents, agent)
		}
		cm.cacheAgents[agent] = true
	}

	klog.Infof("reset cache agents to %v", agents)
	return cm.storage.UpdateRaw(cacheAgentsKey, []byte(strings.Join(agents, sepForAgent)))
}

// UpdateCacheAgents update cache agents
func (cm *cacheManager) UpdateCacheAgents(agents []string) error {
	if len(agents) == 0 {
		klog.Infof("no cache agent is set for update")
		return nil
	}

	hasUpdated := false
	updatedAgents := append(defaultCacheAgents, agents...)
	cm.Lock()
	defer cm.Unlock()
	if len(updatedAgents) != len(cm.cacheAgents) {
		hasUpdated = true
	} else {
		for _, agent := range agents {
			if _, ok := cm.cacheAgents[agent]; !ok {
				hasUpdated = true
				break
			}
		}
	}

	if hasUpdated {
		for k, v := range cm.cacheAgents {
			if !v {
				// not default agent
				delete(cm.cacheAgents, k)
			}
		}

		for _, agent := range agents {
			cm.cacheAgents[agent] = false
		}
		return cm.storage.UpdateRaw(cacheAgentsKey, []byte(strings.Join(updatedAgents, sepForAgent)))
	}
	return nil
}

// ListCacheAgents get all of cache agents
func (cm *cacheManager) ListCacheAgents() []string {
	cm.RLock()
	cm.RUnlock()
	agents := make([]string, 0)
	for k := range cm.cacheAgents {
		agents = append(agents, k)
	}
	return agents
}
