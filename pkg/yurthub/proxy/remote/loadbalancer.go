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

package remote

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	roundRobinStrategy        = "round-robin"
	priorityStrategy          = "priority"
	consistentHashingStrategy = "consistent-hashing"
)

// LoadBalancingStrategy defines the interface for different load balancing strategies.
type LoadBalancingStrategy interface {
	Name() string
	PickOne(req *http.Request) *RemoteProxy
	UpdateBackends(backends []*RemoteProxy)
}

// BaseLoadBalancingStrategy provides common logic for load balancing strategies.
type BaseLoadBalancingStrategy struct {
	sync.RWMutex
	checker  healthchecker.Interface
	backends []*RemoteProxy
}

// UpdateBackends updates the list of backends in a thread-safe manner.
func (b *BaseLoadBalancingStrategy) UpdateBackends(backends []*RemoteProxy) {
	b.Lock()
	defer b.Unlock()
	b.backends = backends
}

// checkAndReturnHealthyBackend checks if a backend is healthy before returning it.
func (b *BaseLoadBalancingStrategy) checkAndReturnHealthyBackend(index int) *RemoteProxy {
	if len(b.backends) == 0 {
		return nil
	}

	backend := b.backends[index]
	if !yurtutil.IsNil(b.checker) && !b.checker.BackendIsHealthy(backend.RemoteServer()) {
		return nil
	}
	return backend
}

// RoundRobinStrategy implements round-robin load balancing.
type RoundRobinStrategy struct {
	BaseLoadBalancingStrategy
	next uint64
}

// Name returns the name of the strategy.
func (rr *RoundRobinStrategy) Name() string {
	return roundRobinStrategy
}

// PickOne selects a backend using a round-robin approach.
func (rr *RoundRobinStrategy) PickOne(_ *http.Request) *RemoteProxy {
	rr.RLock()
	defer rr.RUnlock()

	if len(rr.backends) == 0 {
		return nil
	}

	totalBackends := len(rr.backends)
	// Infinite loop to handle CAS failures and ensure fair selection under high concurrency.
	for {
		// load the current round-robin index.
		startIndex := int(atomic.LoadUint64(&rr.next))
		for i := 0; i < totalBackends; i++ {
			index := (startIndex + i) % totalBackends
			if backend := rr.checkAndReturnHealthyBackend(index); backend != nil {
				// attempt to update next atomically using CAS(Compare-And-Swap)
				// if another go routine has already updated next, CAS operation will fail.
				// if successful, next is updated to index+1 to maintain round-robin fairness.
				if atomic.CompareAndSwapUint64(&rr.next, uint64(startIndex), uint64(index+1)) {
					return backend
				}
				// CAS operation failed, meaning another go routine modified next, so break to retry the selection process.
				break
			}
		}

		// if no healthy backend is found, exit the loop and return nil.
		if !rr.hasHealthyBackend() {
			return nil
		}
	}
}

// hasHealthyBackend checks if there is at least one healthy backend available.
func (rr *RoundRobinStrategy) hasHealthyBackend() bool {
	for i := range rr.backends {
		if rr.checkAndReturnHealthyBackend(i) != nil {
			return true
		}
	}
	return false
}

// PriorityStrategy implements priority-based load balancing.
type PriorityStrategy struct {
	BaseLoadBalancingStrategy
}

// Name returns the name of the strategy.
func (prio *PriorityStrategy) Name() string {
	return priorityStrategy
}

// PickOne selects the first available healthy backend.
func (prio *PriorityStrategy) PickOne(_ *http.Request) *RemoteProxy {
	prio.RLock()
	defer prio.RUnlock()
	for i := 0; i < len(prio.backends); i++ {
		if backend := prio.checkAndReturnHealthyBackend(i); backend != nil {
			return backend
		}
	}

	return nil
}

// ConsistentHashingStrategy implements consistent hashing load balancing.
type ConsistentHashingStrategy struct {
	BaseLoadBalancingStrategy
	nodes  map[uint32]*RemoteProxy
	hashes []uint32
}

// Name returns the name of the strategy.
func (ch *ConsistentHashingStrategy) Name() string {
	return consistentHashingStrategy
}

func (ch *ConsistentHashingStrategy) checkAndReturnHealthyBackend(i int) *RemoteProxy {
	if len(ch.hashes) == 0 {
		return nil
	}

	backend := ch.nodes[ch.hashes[i]]
	if !yurtutil.IsNil(ch.BaseLoadBalancingStrategy.checker) &&
		!ch.BaseLoadBalancingStrategy.checker.BackendIsHealthy(backend.RemoteServer()) {
		return nil
	}
	return backend
}

// PickOne selects a backend using consistent hashing.
func (ch *ConsistentHashingStrategy) PickOne(req *http.Request) *RemoteProxy {
	ch.RLock()
	defer ch.RUnlock()

	if len(ch.hashes) == 0 {
		return nil
	}

	// Calculate the hash of the request
	var firstHealthyBackend *RemoteProxy
	hash := getHash(req.UserAgent() + req.RequestURI)
	for i, h := range ch.hashes {
		// Find the nearest backend with a hash greater than or equal to the request hash
		// return the first healthy backend found
		if h >= hash {
			if backend := ch.checkAndReturnHealthyBackend(i); backend != nil {
				return backend
			}
		}
		// If no backend is found, set the first healthy backend if healthy
		if firstHealthyBackend == nil {
			if backend := ch.checkAndReturnHealthyBackend(i); backend != nil {
				firstHealthyBackend = backend
			}
		}
	}

	// Wrap around
	return firstHealthyBackend
}

func (ch *ConsistentHashingStrategy) UpdateBackends(backends []*RemoteProxy) {
	ch.Lock()
	defer ch.Unlock()

	updatedNodes := make(map[uint32]*RemoteProxy)

	for _, b := range backends {
		nodeHash := getHash(b.Name())
		if _, ok := ch.nodes[nodeHash]; ok {
			// Node already exists
			updatedNodes[nodeHash] = ch.nodes[nodeHash]
			continue
		}

		// New node added
		updatedNodes[nodeHash] = b
	}

	// Sort hash keys
	ch.nodes = updatedNodes
	ch.hashes = slices.Sorted(maps.Keys(updatedNodes))
}

// getHash returns the hash of a string key.
// It uses the FNV-1a algorithm to calculate the hash.
func getHash(key string) uint32 {
	fnvHash := fnv.New32()
	fnvHash.Write([]byte(key))
	return fnvHash.Sum32()
}

// Server is an interface for proxying http request to remote server
// based on the load balance mode(round-robin or priority)
type Server interface {
	UpdateBackends(remoteServers []*url.URL)
	PickOne(req *http.Request) *RemoteProxy
	CurrentStrategy() LoadBalancingStrategy
}

// LoadBalancer is a struct that holds the load balancing strategy and backends.
type LoadBalancer struct {
	strategy      LoadBalancingStrategy
	localCacheMgr cachemanager.CacheManager
	filterFinder  filter.FilterFinder
	transportMgr  transport.TransportManager
	healthChecker healthchecker.Interface
	mode          string
}

// NewLoadBalancer creates a loadbalancer for specified remote servers
func NewLoadBalancer(
	lbMode string,
	remoteServers []*url.URL,
	localCacheMgr cachemanager.CacheManager,
	transportMgr transport.TransportManager,
	healthChecker healthchecker.Interface,
	filterFinder filter.FilterFinder) *LoadBalancer {
	lb := &LoadBalancer{
		mode:          lbMode,
		localCacheMgr: localCacheMgr,
		filterFinder:  filterFinder,
		transportMgr:  transportMgr,
		healthChecker: healthChecker,
	}

	// initialize backends
	lb.UpdateBackends(remoteServers)

	return lb
}

// UpdateBackends dynamically updates the list of remote servers.
func (lb *LoadBalancer) UpdateBackends(remoteServers []*url.URL) {
	newBackends := make([]*RemoteProxy, 0, len(remoteServers))
	for _, server := range remoteServers {
		proxy, err := NewRemoteProxy(server, lb.modifyResponse, lb.errorHandler, lb.transportMgr)
		if err != nil {
			klog.Errorf("could not create proxy for backend %s, %v", server.String(), err)
			continue
		}
		newBackends = append(newBackends, proxy)
	}

	if lb.strategy == nil {
		switch lb.mode {
		case "consistent-hashing":
			lb.strategy = &ConsistentHashingStrategy{
				BaseLoadBalancingStrategy: BaseLoadBalancingStrategy{checker: lb.healthChecker},
				nodes:                     make(map[uint32]*RemoteProxy),
				hashes:                    make([]uint32, 0, len(newBackends)),
			}
		case "priority":
			lb.strategy = &PriorityStrategy{BaseLoadBalancingStrategy{checker: lb.healthChecker}}
		default:
			lb.strategy = &RoundRobinStrategy{BaseLoadBalancingStrategy{checker: lb.healthChecker}, 0}
		}
	}

	lb.strategy.UpdateBackends(newBackends)
}

func (lb *LoadBalancer) PickOne(req *http.Request) *RemoteProxy {
	return lb.strategy.PickOne(req)
}

func (lb *LoadBalancer) CurrentStrategy() LoadBalancingStrategy {
	return lb.strategy
}

// errorHandler handles errors and tries to serve from local cache.
func (lb *LoadBalancer) errorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	klog.Errorf("remote proxy error handler: %s, %v", hubutil.ReqString(req), err)
	if lb.localCacheMgr == nil || !lb.localCacheMgr.CanCacheFor(req) {
		rw.WriteHeader(http.StatusBadGateway)
		return
	}

	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		if info.Verb == "get" || info.Verb == "list" {
			if obj, err := lb.localCacheMgr.QueryCache(req); err == nil {
				hubutil.WriteObject(http.StatusOK, obj, rw, req)
				return
			}
		}
	}
	rw.WriteHeader(http.StatusBadGateway)
}

func (lb *LoadBalancer) modifyResponse(resp *http.Response) error {
	if resp == nil || resp.Request == nil {
		klog.Infof("no request info in response, skip cache response")
		return nil
	}

	req := resp.Request
	ctx := req.Context()

	// re-added transfer-encoding=chunked response header for watch request
	info, exists := apirequest.RequestInfoFrom(ctx)
	if exists {
		if info.Verb == "watch" {
			klog.V(5).Infof("add transfer-encoding=chunked header into response for req %s", hubutil.ReqString(req))
			h := resp.Header
			if hv := h.Get("Transfer-Encoding"); hv == "" {
				h.Add("Transfer-Encoding", "chunked")
			}
		}
	}

	// wrap response for tracing traffic information of requests
	resp = hubutil.WrapWithTrafficTrace(req, resp)

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusPartialContent {
		// prepare response content type
		reqContentType, _ := hubutil.ReqContentTypeFrom(ctx)
		respContentType := resp.Header.Get(yurtutil.HttpHeaderContentType)
		if len(respContentType) == 0 {
			respContentType = reqContentType
		}
		ctx = hubutil.WithRespContentType(ctx, respContentType)
		req = req.WithContext(ctx)

		// filter response data
		if !yurtutil.IsNil(lb.filterFinder) {
			if responseFilter, ok := lb.filterFinder.FindResponseFilter(req); ok {
				wrapBody, needUncompressed := hubutil.NewGZipReaderCloser(resp.Header, resp.Body, req, "filter")
				size, filterRc, err := responseFilter.Filter(req, wrapBody)
				if err != nil {
					klog.Errorf("could not filter response for %s, %v", hubutil.ReqString(req), err)
					return err
				}
				resp.Body = filterRc
				if size > 0 {
					resp.ContentLength = int64(size)
					resp.Header.Set(yurtutil.HttpHeaderContentLength, fmt.Sprint(size))
				}

				// after gunzip in filter, the header content encoding should be removed.
				// because there's no need to gunzip response.body again.
				if needUncompressed {
					resp.Header.Del("Content-Encoding")
				}
			}
		}

		if !yurtutil.IsNil(lb.localCacheMgr) {
			// cache resp with storage interface
			lb.cacheResponse(req, resp)
		}
	} else if resp.StatusCode == http.StatusNotFound && info.Verb == "list" && lb.localCacheMgr != nil {
		// 404 Not Found: The CRD may have been unregistered and should be updated locally as well.
		// Other types of requests may return a 404 response for other reasons (for example, getting a pod that doesn't exist).
		// And the main purpose is to return 404 when list an unregistered resource locally, so here only consider the list request.
		gvr := schema.GroupVersionResource{
			Group:    info.APIGroup,
			Version:  info.APIVersion,
			Resource: info.Resource,
		}

		err := lb.localCacheMgr.DeleteKindFor(gvr)
		if err != nil {
			klog.Errorf("failed: %v", err)
		}
	}
	return nil
}

func (lb *LoadBalancer) cacheResponse(req *http.Request, resp *http.Response) {
	if lb.localCacheMgr.CanCacheFor(req) {
		wrapPrc, needUncompressed := hubutil.NewGZipReaderCloser(resp.Header, resp.Body, req, "cache-manager")
		// after gunzip in filter, the header content encoding should be removed.
		// because there's no need to gunzip response.body again.
		if needUncompressed {
			resp.Header.Del("Content-Encoding")
		}
		resp.Body = wrapPrc

		// cache the response at local.
		rc, prc := hubutil.NewDualReadCloser(req, resp.Body, true)
		go func(req *http.Request, prc io.ReadCloser) {
			if err := lb.localCacheMgr.CacheResponse(req, prc); err != nil && !errors.Is(err, io.EOF) &&
				!errors.Is(err, context.Canceled) {
				klog.Errorf("lb could not cache req %s in local cache, %v", hubutil.ReqString(req), err)
			}
		}(req, prc)
		resp.Body = rc
	}
}
