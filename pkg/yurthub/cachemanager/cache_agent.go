package cachemanager

import (
	"net/http"
	"strings"

	"github.com/alibaba/openyurt/pkg/yurthub/util"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog"
)

var (
	ResourceToKindMap = map[string]string{
		"nodes":                  "Node",
		"pods":                   "Pod",
		"services":               "Service",
		"namespaces":             "Namespace",
		"endpoints":              "Endpoints",
		"configmaps":             "ConfigMap",
		"persistentvolumes":      "PersistentVolume",
		"persistentvolumeclaims": "PersistentVolumeClaim",
		"events":                 "Event",
		"secrets":                "Secret",
		"leases":                 "Lease",
		"runtimeclasses":         "RuntimeClass",
		"csidrivers":             "CSIDriver",
	}

	ResourceToListKindMap = map[string]string{
		"nodes":                  "NodeList",
		"pods":                   "PodList",
		"services":               "ServiceList",
		"namespaces":             "NamespaceList",
		"endpoints":              "EndpointsList",
		"configmaps":             "ConfigMapList",
		"persistentvolumes":      "PersistentVolumeList",
		"persistentvolumeclaims": "PersistentVolumeClaimList",
		"events":                 "EventList",
		"secrets":                "SecretList",
		"leases":                 "LeaseList",
		"runtimeclasses":         "RuntimeClassList",
		"csidrivers":             "CSIDriverList",
	}

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

func (ecm *cacheManager) initCacheAgents() error {
	agents := make([]string, 0)
	b, err := ecm.storage.GetRaw(cacheAgentsKey)
	if err == nil && len(b) != 0 {
		localAgents := strings.Split(string(b), sepForAgent)
		if len(localAgents) < len(defaultCacheAgents) {
			err = ecm.storage.Delete(cacheAgentsKey)
			if err != nil {
				klog.Errorf("failed to delete agents cache, %v", err)
				return err
			}
		} else {
			agents = append(agents, localAgents...)
			for _, agent := range localAgents {
				ecm.cacheAgents[agent] = false
			}
		}
	}
	for _, agent := range defaultCacheAgents {
		if ecm.cacheAgents == nil {
			ecm.cacheAgents = make(map[string]bool)
		}

		if _, ok := ecm.cacheAgents[agent]; !ok {
			agents = append(agents, agent)
		}
		ecm.cacheAgents[agent] = true
	}

	klog.Infof("reset cache agents to %v", agents)
	return ecm.storage.UpdateRaw(cacheAgentsKey, []byte(strings.Join(agents, sepForAgent)))
}

func (ecm *cacheManager) UpdateCacheAgents(agents []string) error {
	if len(agents) == 0 {
		klog.Infof("no cache agent is set for update")
		return nil
	}

	hasUpdated := false
	updatedAgents := append(defaultCacheAgents, agents...)
	ecm.Lock()
	defer ecm.Unlock()
	if len(updatedAgents) != len(ecm.cacheAgents) {
		hasUpdated = true
	} else {
		for _, agent := range agents {
			if _, ok := ecm.cacheAgents[agent]; !ok {
				hasUpdated = true
				break
			}
		}
	}

	if hasUpdated {
		for k, v := range ecm.cacheAgents {
			if !v {
				// not default agent
				delete(ecm.cacheAgents, k)
			}
		}

		for _, agent := range agents {
			ecm.cacheAgents[agent] = false
		}
		return ecm.storage.UpdateRaw(cacheAgentsKey, []byte(strings.Join(updatedAgents, sepForAgent)))
	}
	return nil
}

func (ecm *cacheManager) ListCacheAgents() []string {
	ecm.RLock()
	ecm.RUnlock()
	agents := make([]string, 0)
	for k := range ecm.cacheAgents {
		agents = append(agents, k)
	}
	return agents
}

// CanCacheFor checks response of request can be cached or not
// the following request is not supported to cache response
// 1. component is not set
// 2. delete/deletecollection/proxy request
// 3. sub-resource request but is not status
// 4. csr resource request
func (ecm *cacheManager) CanCacheFor(req *http.Request) bool {
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok || len(comp) == 0 {
		return false
	} else {
		canCache, ok := util.ReqCanCacheFrom(ctx)
		if ok && canCache {
			// request with Edge-Cache header, continue verification
		} else {
			ecm.RLock()
			if _, found := ecm.cacheAgents[comp]; !found {
				ecm.RUnlock()
				return false
			}
			ecm.RUnlock()
		}
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || info == nil {
		return false
	}

	if !info.IsResourceRequest {
		return false
	}

	if info.Verb == "delete" || info.Verb == "deletecollection" || info.Verb == "proxy" {
		return false
	}

	if info.Subresource != "" && info.Subresource != "status" {
		return false
	}

	if _, ok := ResourceToKindMap[info.Resource]; !ok {
		return false
	}

	return true
}
