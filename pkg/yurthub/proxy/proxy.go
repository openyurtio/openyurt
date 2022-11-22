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

package proxy

import (
	"net/http"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/local"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/pool"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type yurtReverseProxy struct {
	resolver                apirequest.RequestInfoResolver
	loadBalancer            remote.LoadBalancer
	cloudHealthChecker      healthchecker.MultipleBackendsHealthChecker
	coordinatorHealtChecker healthchecker.HealthChecker
	localProxy              http.Handler
	poolProxy               http.Handler
	maxRequestsInFlight     int
	tenantMgr               tenant.Interface
	coordinator             *poolcoordinator.Coordinator
	workingMode             hubutil.WorkingMode
}

// NewYurtReverseProxyHandler creates a http handler for proxying
// all of incoming requests.
func NewYurtReverseProxyHandler(
	yurtHubCfg *config.YurtHubConfiguration,
	localCacheMgr cachemanager.CacheManager,
	transportMgr transport.Interface,
	coordinator *poolcoordinator.Coordinator,
	cloudHealthChecker healthchecker.MultipleBackendsHealthChecker,
	coordinatorHealthChecker healthchecker.HealthChecker,
	tenantMgr tenant.Interface,
	stopCh <-chan struct{}) (http.Handler, error) {
	cfg := &server.Config{
		LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
	}
	resolver := server.NewRequestInfoResolver(cfg)

	lb, err := remote.NewLoadBalancer(
		yurtHubCfg.LBMode,
		yurtHubCfg.RemoteServers,
		localCacheMgr,
		transportMgr,
		coordinator,
		cloudHealthChecker,
		yurtHubCfg.FilterManager,
		yurtHubCfg.WorkingMode,
		stopCh)
	if err != nil {
		return nil, err
	}

	var localProxy, poolProxy http.Handler

	if yurtHubCfg.WorkingMode == hubutil.WorkingModeEdge {
		// When yurthub works in Edge mode, we may use local proxy or pool proxy to handle
		// the request when offline.
		localProxy = local.NewLocalProxy(localCacheMgr, cloudHealthChecker.IsHealthy, yurtHubCfg.MinRequestTimeout)
		localProxy = local.WithFakeTokenInject(localProxy, yurtHubCfg.SerializerManager)
		poolProxy, err = pool.NewPoolCoordinatorProxy(
			yurtHubCfg.CoordinatorServer,
			localCacheMgr,
			transportMgr,
			yurtHubCfg.FilterManager,
			func() bool {
				_, isReady := coordinator.IsReady()
				return isReady
			},
			stopCh)
		if err != nil {
			return nil, err
		}
	}

	yurtProxy := &yurtReverseProxy{
		resolver:                resolver,
		loadBalancer:            lb,
		cloudHealthChecker:      cloudHealthChecker,
		coordinatorHealtChecker: coordinatorHealthChecker,
		localProxy:              localProxy,
		poolProxy:               poolProxy,
		maxRequestsInFlight:     yurtHubCfg.MaxRequestInFlight,
		coordinator:             coordinator,
		tenantMgr:               tenantMgr,
		workingMode:             yurtHubCfg.WorkingMode,
	}

	return yurtProxy.buildHandlerChain(yurtProxy), nil
}

func (p *yurtReverseProxy) buildHandlerChain(handler http.Handler) http.Handler {
	handler = util.WithRequestTrace(handler)
	handler = util.WithRequestContentType(handler)
	if p.workingMode == hubutil.WorkingModeEdge {
		handler = util.WithCacheHeaderCheck(handler)
	}
	handler = util.WithRequestTimeout(handler)
	if p.workingMode == hubutil.WorkingModeEdge {
		handler = util.WithListRequestSelector(handler)
	}
	handler = util.WithRequestTraceFull(handler)
	handler = util.WithMaxInFlightLimit(handler, p.maxRequestsInFlight)
	handler = util.WithRequestClientComponent(handler)
	handler = util.WithIfPoolScopedResource(handler)

	if p.tenantMgr != nil && p.tenantMgr.GetTenantNs() != "" {
		handler = util.WithSaTokenSubstitute(handler, p.tenantMgr)
	} else {
		klog.V(2).Info("tenant ns is empty, no need to substitute ")
	}

	handler = filters.WithRequestInfo(handler, p.resolver)

	return handler
}

func (p *yurtReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if util.IsKubeletLeaseReq(req) {
		p.handleKubeletLease(rw, req)
		return
	}

	if p.workingMode == hubutil.WorkingModeCloud {
		p.loadBalancer.ServeHTTP(rw, req)
		return
	}

	if util.IsEventCreateRequest(req) ||
		util.IsPoolScopedResouceListWatchRequest(req) ||
		util.IsSubjectAccessReviewCreateGetRequest(req) {
		// For pool-scoped request, we should check if pool-coordinator is ready for request.
		// We may handle two kinds of request through pool-coordinator:
		// 1. list/watch request for pool-scoped resources for traffic multiplexing.
		// 2. create/get subjectaccessreview resources for enabling logs/exec through pool-coordinator.
		// 3. creat event resources for maintenance
		// We assume that if pool-coordinator is not enabled or is not healthy, IsReady will return false.
		if _, isReady := p.coordinator.IsReady(); isReady {
			p.poolProxy.ServeHTTP(rw, req)
			return
		}
	}

	// For request that do not need to be handled by pool-coordinator, or
	// pool-coordinator is disabled or unhealthy, fall through the normal case, which
	// handling the request with cloud apiserver or local cache.
	if p.cloudHealthChecker.IsHealthy() {
		p.loadBalancer.ServeHTTP(rw, req)
	} else {
		p.localProxy.ServeHTTP(rw, req)
	}
}

func (p *yurtReverseProxy) handleKubeletLease(rw http.ResponseWriter, req *http.Request) {
	p.cloudHealthChecker.RenewKubeletLeaseTime()
	p.coordinatorHealtChecker.RenewKubeletLeaseTime()
	if p.localProxy != nil {
		p.localProxy.ServeHTTP(rw, req)
	} else {
		// Only in cloud mode, poolProxy and localProxy can both be nil.
		// So we should proxy the kubelet lease request with lb.
		p.loadBalancer.ServeHTTP(rw, req)
	}
}
