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
	"io"
	"net/http"
	"net/url"
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

// Backend defines the interface of proxy for a remote backend server.
type Backend interface {
	Name() string
	RemoteServer() *url.URL
	ServeHTTP(rw http.ResponseWriter, req *http.Request)
}

// LoadBalancingStrategy defines the interface for different load balancing strategies.
type LoadBalancingStrategy interface {
	Name() string
	PickOne() Backend
	UpdateBackends(backends []Backend)
}

// BaseLoadBalancingStrategy provides common logic for load balancing strategies.
type BaseLoadBalancingStrategy struct {
	sync.RWMutex
	checker  healthchecker.Interface
	backends []Backend
}

// UpdateBackends updates the list of backends in a thread-safe manner.
func (b *BaseLoadBalancingStrategy) UpdateBackends(backends []Backend) {
	b.Lock()
	defer b.Unlock()
	b.backends = backends
}

// checkAndReturnHealthyBackend checks if a backend is healthy before returning it.
func (b *BaseLoadBalancingStrategy) checkAndReturnHealthyBackend(index int) Backend {
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
	return "round-robin"
}

// PickOne selects a backend using a round-robin approach.
func (rr *RoundRobinStrategy) PickOne() Backend {
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
	return "priority"
}

// PickOne selects the first available healthy backend.
func (prio *PriorityStrategy) PickOne() Backend {
	prio.RLock()
	defer prio.RUnlock()
	for i := 0; i < len(prio.backends); i++ {
		if backend := prio.checkAndReturnHealthyBackend(i); backend != nil {
			return backend
		}
	}

	return nil
}

// LoadBalancer is an interface for proxying http request to remote server
// based on the load balance mode(round-robin or priority)
type LoadBalancer interface {
	UpdateBackends(remoteServers []*url.URL)
	PickOne() Backend
	CurrentStrategy() LoadBalancingStrategy
}

type loadBalancer struct {
	strategy      LoadBalancingStrategy
	localCacheMgr cachemanager.CacheManager
	filterFinder  filter.FilterFinder
	transportMgr  transport.Interface
	healthChecker healthchecker.Interface
	mode          string
	stopCh        <-chan struct{}
}

// NewLoadBalancer creates a loadbalancer for specified remote servers
func NewLoadBalancer(
	lbMode string,
	remoteServers []*url.URL,
	localCacheMgr cachemanager.CacheManager,
	transportMgr transport.Interface,
	healthChecker healthchecker.Interface,
	filterFinder filter.FilterFinder,
	stopCh <-chan struct{}) LoadBalancer {
	lb := &loadBalancer{
		mode:          lbMode,
		localCacheMgr: localCacheMgr,
		filterFinder:  filterFinder,
		transportMgr:  transportMgr,
		healthChecker: healthChecker,
		stopCh:        stopCh,
	}

	// initialize backends
	lb.UpdateBackends(remoteServers)

	return lb
}

// UpdateBackends dynamically updates the list of remote servers.
func (lb *loadBalancer) UpdateBackends(remoteServers []*url.URL) {
	newBackends := make([]Backend, 0, len(remoteServers))
	for _, server := range remoteServers {
		proxy, err := NewRemoteProxy(server, lb.modifyResponse, lb.errorHandler, lb.transportMgr, lb.stopCh)
		if err != nil {
			klog.Errorf("could not create proxy for backend %s, %v", server.String(), err)
			continue
		}
		newBackends = append(newBackends, proxy)
	}

	if lb.strategy == nil {
		switch lb.mode {
		case "priority":
			lb.strategy = &PriorityStrategy{BaseLoadBalancingStrategy{checker: lb.healthChecker}}
		default:
			lb.strategy = &RoundRobinStrategy{BaseLoadBalancingStrategy{checker: lb.healthChecker}, 0}
		}
	}

	lb.strategy.UpdateBackends(newBackends)
}

func (lb *loadBalancer) PickOne() Backend {
	return lb.strategy.PickOne()
}

func (lb *loadBalancer) CurrentStrategy() LoadBalancingStrategy {
	return lb.strategy
}

// errorHandler handles errors and tries to serve from local cache.
func (lb *loadBalancer) errorHandler(rw http.ResponseWriter, req *http.Request, err error) {
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

func (lb *loadBalancer) modifyResponse(resp *http.Response) error {
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
				size, filterRc, err := responseFilter.Filter(req, wrapBody, lb.stopCh)
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

func (lb *loadBalancer) cacheResponse(req *http.Request, resp *http.Response) {
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
		go func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) {
			if err := lb.localCacheMgr.CacheResponse(req, prc, stopCh); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				klog.Errorf("lb could not cache req %s in local cache, %v", hubutil.ReqString(req), err)
			}
		}(req, prc, req.Context().Done())
		resp.Body = rc
	}
}
