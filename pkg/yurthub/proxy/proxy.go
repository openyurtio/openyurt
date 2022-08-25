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
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/local"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type yurtReverseProxy struct {
	resolver            apirequest.RequestInfoResolver
	loadBalancer        remote.LoadBalancer
	checker             healthchecker.HealthChecker
	localProxy          http.Handler
	cacheMgr            cachemanager.CacheManager
	maxRequestsInFlight int
	tenantMgr           tenant.Interface
	stopCh              <-chan struct{}
}

// NewYurtReverseProxyHandler creates a http handler for proxying
// all of incoming requests.
func NewYurtReverseProxyHandler(
	yurtHubCfg *config.YurtHubConfiguration,
	cacheMgr cachemanager.CacheManager,
	transportMgr transport.Interface,
	healthChecker healthchecker.HealthChecker,
	certManager interfaces.YurtCertificateManager,
	tenantMgr tenant.Interface,
	stopCh <-chan struct{}) (http.Handler, error) {
	cfg := &server.Config{
		LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
	}
	resolver := server.NewRequestInfoResolver(cfg)

	lb, err := remote.NewLoadBalancer(
		yurtHubCfg.LBMode,
		yurtHubCfg.RemoteServers,
		cacheMgr,
		transportMgr,
		healthChecker,
		certManager,
		yurtHubCfg.FilterManager,
		stopCh)
	if err != nil {
		return nil, err
	}

	var localProxy http.Handler
	// When yurthub is working in cloud mode, cacheMgr will be set to nil which means the local cache is disabled,
	// so we don't need to create a LocalProxy.
	if cacheMgr != nil {
		localProxy = local.NewLocalProxy(cacheMgr, lb.IsHealthy)
		localProxy = local.WithFakeTokenInject(localProxy, yurtHubCfg.SerializerManager)
	}

	yurtProxy := &yurtReverseProxy{
		resolver:            resolver,
		loadBalancer:        lb,
		checker:             healthChecker,
		localProxy:          localProxy,
		cacheMgr:            cacheMgr,
		maxRequestsInFlight: yurtHubCfg.MaxRequestInFlight,
		tenantMgr:           tenantMgr,
		stopCh:              stopCh,
	}

	return yurtProxy.buildHandlerChain(yurtProxy), nil
}

func (p *yurtReverseProxy) buildHandlerChain(handler http.Handler) http.Handler {
	handler = util.WithRequestTrace(handler)
	handler = util.WithRequestContentType(handler)
	if p.cacheMgr != nil {
		handler = util.WithCacheHeaderCheck(handler)
	}
	handler = util.WithRequestTimeout(handler)
	if p.cacheMgr != nil {
		handler = util.WithListRequestSelector(handler)
	}
	handler = util.WithRequestTraceFull(handler)
	handler = util.WithMaxInFlightLimit(handler, p.maxRequestsInFlight)
	handler = util.WithRequestClientComponent(handler)

	if p.tenantMgr != nil && p.tenantMgr.GetTenantNs() != "" {
		handler = util.WithSaTokenSubstitute(handler, p.tenantMgr)
	} else {
		klog.V(2).Info("tenant ns is empty, no need to substitute ")
	}

	handler = filters.WithRequestInfo(handler, p.resolver)

	return handler
}

func (p *yurtReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	isKubeletLeaseReq := hubutil.IsKubeletLeaseReq(req)
	if !isKubeletLeaseReq && p.loadBalancer.IsHealthy() || p.localProxy == nil {
		p.loadBalancer.ServeHTTP(rw, req)
	} else {
		if isKubeletLeaseReq {
			p.checker.UpdateLastKubeletLeaseReqTime(time.Now())
		}
		p.localProxy.ServeHTTP(rw, req)
	}
}
