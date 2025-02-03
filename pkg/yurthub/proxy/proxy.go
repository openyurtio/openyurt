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
	"errors"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	hubrest "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/autonomy"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/local"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const multiplexerProxyPostHookName = "multiplexerProxy"

type yurtReverseProxy struct {
	resolver             apirequest.RequestInfoResolver
	loadBalancer         remote.LoadBalancer
	cloudHealthChecker   healthchecker.MultipleBackendsHealthChecker
	localProxy           http.Handler
	autonomyProxy        http.Handler
	maxRequestsInFlight  int
	tenantMgr            tenant.Interface
	workingMode          hubutil.WorkingMode
	multiplexerProxy     http.Handler
	multiplexerResources []schema.GroupVersionResource
	nodeName             string
}

// NewYurtReverseProxyHandler creates a http handler for proxying
// all of incoming requests.
func NewYurtReverseProxyHandler(
	yurtHubCfg *config.YurtHubConfiguration,
	localCacheMgr cachemanager.CacheManager,
	restConfigMgr *hubrest.RestConfigManager,
	transportMgr transport.Interface,
	cloudHealthChecker healthchecker.MultipleBackendsHealthChecker,
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
		cloudHealthChecker,
		yurtHubCfg.FilterFinder,
		yurtHubCfg.WorkingMode,
		stopCh)
	if err != nil {
		return nil, err
	}

	var localProxy, autonomyProxy http.Handler

	if yurtHubCfg.WorkingMode == hubutil.WorkingModeEdge {
		// When yurthub works in Edge mode, we may use local proxy or pool proxy to handle
		// the request when offline.
		localProxy = local.NewLocalProxy(localCacheMgr,
			cloudHealthChecker.IsHealthy,
			yurtHubCfg.MinRequestTimeout,
		)
		localProxy = local.WithFakeTokenInject(localProxy, yurtHubCfg.SerializerManager)

		autonomyProxy = autonomy.NewAutonomyProxy(
			restConfigMgr,
			localCacheMgr,
		)
	}

	yurtProxy := &yurtReverseProxy{
		resolver:             resolver,
		loadBalancer:         lb,
		cloudHealthChecker:   cloudHealthChecker,
		localProxy:           localProxy,
		autonomyProxy:        autonomyProxy,
		maxRequestsInFlight:  yurtHubCfg.MaxRequestInFlight,
		tenantMgr:            tenantMgr,
		workingMode:          yurtHubCfg.WorkingMode,
		multiplexerResources: yurtHubCfg.MultiplexerResources,
		nodeName:             yurtHubCfg.NodeName,
	}

	if yurtHubCfg.PostStartHooks == nil {
		yurtHubCfg.PostStartHooks = make(map[string]func() error)
	}
	yurtHubCfg.PostStartHooks[multiplexerProxyPostHookName] = func() error {
		if yurtProxy.multiplexerProxy, err = multiplexer.NewMultiplexerProxy(yurtHubCfg.FilterFinder,
			yurtHubCfg.RequestMultiplexerManager,
			yurtHubCfg.MultiplexerResources,
			stopCh); err != nil {
			return fmt.Errorf("failed to new default share proxy, error: %v", err)
		}
		return nil
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
	handler = util.WithRequestClientComponent(handler, p.workingMode)
	handler = util.WithPartialObjectMetadataRequest(handler)

	if p.tenantMgr != nil && p.tenantMgr.GetTenantNs() != "" {
		handler = util.WithSaTokenSubstitute(handler, p.tenantMgr)
	} else {
		klog.V(2).Info("tenant ns is empty, no need to substitute ")
	}

	handler = filters.WithRequestInfo(handler, p.resolver)

	return handler
}

func (p *yurtReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if p.workingMode == hubutil.WorkingModeCloud {
		p.loadBalancer.ServeHTTP(rw, req)
		return
	}

	switch {
	case util.IsKubeletLeaseReq(req):
		p.handleKubeletLease(rw, req)
	case util.IsKubeletGetNodeReq(req):
		if p.autonomyProxy != nil {
			p.autonomyProxy.ServeHTTP(rw, req)
		} else {
			p.loadBalancer.ServeHTTP(rw, req)
		}
	case util.IsEventCreateRequest(req):
		p.eventHandler(rw, req)
	case util.IsSubjectAccessReviewCreateGetRequest(req):
		p.subjectAccessReviewHandler(rw, req)
	case util.IsMultiplexerRequest(req, p.multiplexerResources, p.nodeName):
		if p.multiplexerProxy != nil {
			p.multiplexerProxy.ServeHTTP(rw, req)
		}
	default:
		// handling the request with cloud apiserver or local cache.
		if p.cloudHealthChecker.IsHealthy() {
			p.loadBalancer.ServeHTTP(rw, req)
		} else {
			p.localProxy.ServeHTTP(rw, req)
		}
	}
}

func (p *yurtReverseProxy) handleKubeletLease(rw http.ResponseWriter, req *http.Request) {
	p.cloudHealthChecker.RenewKubeletLeaseTime()

	if p.localProxy != nil {
		p.localProxy.ServeHTTP(rw, req)
	}
}

func (p *yurtReverseProxy) eventHandler(rw http.ResponseWriter, req *http.Request) {
	if p.cloudHealthChecker.IsHealthy() {
		p.loadBalancer.ServeHTTP(rw, req)
	} else {
		p.localProxy.ServeHTTP(rw, req)
	}
}

func (p *yurtReverseProxy) subjectAccessReviewHandler(rw http.ResponseWriter, req *http.Request) {
	if p.cloudHealthChecker.IsHealthy() {
		p.loadBalancer.ServeHTTP(rw, req)
	} else {
		err := errors.New("request is from cloud APIServer but it's currently not healthy")
		klog.Errorf("could not handle SubjectAccessReview req %s, %v", hubutil.ReqString(req), err)
		hubutil.Err(err, rw, req)
	}
}
