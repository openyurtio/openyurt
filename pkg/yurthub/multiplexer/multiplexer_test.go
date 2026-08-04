/*
Copyright 2026 The OpenYurt Authors.

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

package multiplexer

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
)

type dummyHealthChecker struct{}

func (d *dummyHealthChecker) RenewKubeletLeaseTime()                   {}
func (d *dummyHealthChecker) IsHealthy() bool                          { return true }
func (d *dummyHealthChecker) BackendIsHealthy(server *url.URL) bool     { return true }
func (d *dummyHealthChecker) PickOneHealthyBackend() *url.URL          { return nil }
func (d *dummyHealthChecker) UpdateBackends(servers []*url.URL)        {}

type dummyLoadBalancer struct{}

func (d *dummyLoadBalancer) UpdateBackends(remoteServers []*url.URL) {}
func (d *dummyLoadBalancer) PickOne(req *http.Request) *remote.RemoteProxy {
	return nil
}
func (d *dummyLoadBalancer) CurrentStrategy() remote.LoadBalancingStrategy {
	return nil
}

func TestMultiplexerManager_TombstoneAndNilHandling(t *testing.T) {
	m := &MultiplexerManager{
		lazyLoadedGVRCache:            make(map[string]Interface),
		lazyLoadedGVRCacheDestroyFunc: make(map[string]func()),
		poolScopeMetadata:             sets.New[string](),
		leaderAddresses:               sets.New[string](),
		healthCheckerForLeaders:       &dummyHealthChecker{},
		loadBalancerForLeaders:        &dummyLoadBalancer{},
	}

	// 1. Should handle nil object gracefully
	assert.NotPanics(t, func() {
		m.addConfigmap(nil)
		m.updateConfigmap(nil, nil)
	})

	// 2. Should handle non-ConfigMap invalid types gracefully
	assert.NotPanics(t, func() {
		m.addConfigmap("invalid-string-obj")
		m.updateConfigmap("invalid-old", "invalid-new")
	})

	// 3. Should handle DeletedFinalStateUnknown tombstone gracefully
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/leader-hub-pool1",
		Obj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "leader-hub-pool1",
				Namespace: "default",
			},
			Data: map[string]string{
				PoolScopeMetadataKey: "apps/v1/deployments",
			},
		},
	}

	assert.NotPanics(t, func() {
		m.addConfigmap(tombstone)
	})
	assert.Equal(t, 1, m.poolScopeMetadata.Len())
	assert.True(t, m.poolScopeMetadata.Has("apps/v1/deployments"))
}

func TestMultiplexerManager_FilterStoreCleanupOnGVRDeletion(t *testing.T) {
	fsm := &filterStoreManager{
		filterStores: make(map[string]*filterStore),
	}
	fsm.filterStores["apps/v1/deployments"] = &filterStore{}

	m := &MultiplexerManager{
		filterStoreManager:            fsm,
		lazyLoadedGVRCache:            make(map[string]Interface),
		lazyLoadedGVRCacheDestroyFunc: make(map[string]func()),
		poolScopeMetadata:             sets.New("apps/v1/deployments"),
		leaderAddresses:               sets.New[string](),
		healthCheckerForLeaders:       &dummyHealthChecker{},
		loadBalancerForLeaders:        &dummyLoadBalancer{},
	}

	// Update ConfigMap without deployments
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			PoolScopeMetadataKey: "",
		},
	}

	m.updateLeaderHubConfiguration(cm)

	// Verify filterStore for apps/v1/deployments was purged
	assert.False(t, m.poolScopeMetadata.Has("apps/v1/deployments"))
	fsm.RLock()
	_, exists := fsm.filterStores["apps/v1/deployments"]
	fsm.RUnlock()
	assert.False(t, exists, "filterStore for removed GVR should be purged from filterStoreManager")
}

func TestMultiplexerManager_ConcurrentConfigMapUpdateAndResolution(t *testing.T) {
	m := &MultiplexerManager{
		lazyLoadedGVRCache:            make(map[string]Interface),
		lazyLoadedGVRCacheDestroyFunc: make(map[string]func()),
		poolScopeMetadata:             sets.New("apps/v1/deployments", "core/v1/services"),
		leaderAddresses:               sets.New[string](),
		healthCheckerForLeaders:       &dummyHealthChecker{},
		loadBalancerForLeaders:        &dummyLoadBalancer{},
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Goroutine 1: Rapidly updates ConfigMap
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				cm := &corev1.ConfigMap{
					Data: map[string]string{
						PoolScopeMetadataKey: "apps/v1/deployments,core/v1/pods",
						LeaderEndpointsKey:   "node1/10.0.0.1",
						EnableLeaderElection: "true",
					},
				}
				m.updateLeaderHubConfiguration(cm)
			}
		}
	}()

	// Goroutine 2: Rapidly reads source and resolves requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = m.SourceForPoolScopeMetadata()

				req, _ := http.NewRequest("GET", "/api/v1/pods", nil)
				reqInfo := &apirequest.RequestInfo{
					Verb:       "list",
					APIGroup:   "",
					APIVersion: "v1",
					Resource:   "pods",
				}
				req = req.WithContext(apirequest.WithRequestInfo(req.Context(), reqInfo))
				_, _ = m.ResolveRequestForPoolScopeMetadata(req)
			}
		}
	}()

	wg.Wait()
}
