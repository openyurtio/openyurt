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
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	basemultiplexer "github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/autonomy"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/local"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/nonresourcerequest"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/remote"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type yurtReverseProxy struct {
	cfg                      *config.YurtHubConfiguration
	cloudHealthChecker       healthchecker.Interface
	resolver                 apirequest.RequestInfoResolver
	loadBalancer             remote.LoadBalancer
	loadBalancerForLeaderHub remote.LoadBalancer
	localProxy               http.Handler
	autonomyProxy            http.Handler
	multiplexerProxy         http.Handler
	multiplexerManager       *basemultiplexer.MultiplexerManager
	tenantMgr                tenant.Interface
	nodeName                 string
	multiplexerUserAgent     string
}

// NewYurtReverseProxyHandler creates a http handler for proxying
// all of incoming requests.
func NewYurtReverseProxyHandler(
	yurtHubCfg *config.YurtHubConfiguration,
	localCacheMgr cachemanager.CacheManager,
	cloudHealthChecker healthchecker.Interface,
	requestMultiplexerManager *basemultiplexer.MultiplexerManager,
	stopCh <-chan struct{}) (http.Handler, error) {
	cfg := &server.Config{
		LegacyAPIGroupPrefixes: sets.NewString(server.DefaultLegacyAPIPrefix),
	}
	resolver := server.NewRequestInfoResolver(cfg)

	lb := remote.NewLoadBalancer(
		yurtHubCfg.LBMode,
		yurtHubCfg.RemoteServers,
		localCacheMgr,
		yurtHubCfg.TransportAndDirectClientManager,
		cloudHealthChecker,
		yurtHubCfg.FilterFinder,
		stopCh)

	var localProxy, autonomyProxy http.Handler
	if !yurtutil.IsNil(cloudHealthChecker) && !yurtutil.IsNil(localCacheMgr) {
		// When yurthub works in Edge mode, health checker and cache manager are prepared.
		// so we may use local proxy and autonomy proxy to handle the request when offline.
		localProxy = local.NewLocalProxy(localCacheMgr,
			cloudHealthChecker.IsHealthy,
			yurtHubCfg.MinRequestTimeout,
		)
		localProxy = local.WithFakeTokenInject(localProxy, yurtHubCfg.SerializerManager)

		autonomyProxy = autonomy.NewAutonomyProxy(
			cloudHealthChecker,
			yurtHubCfg.TransportAndDirectClientManager,
			localCacheMgr,
		)
	}

	multiplexerProxy := multiplexer.NewMultiplexerProxy(yurtHubCfg.FilterFinder,
		requestMultiplexerManager,
		yurtHubCfg.RESTMapperManager,
		stopCh)

	yurtProxy := &yurtReverseProxy{
		cfg:                      yurtHubCfg,
		resolver:                 resolver,
		loadBalancer:             lb,
		loadBalancerForLeaderHub: yurtHubCfg.LoadBalancerForLeaderHub,
		cloudHealthChecker:       cloudHealthChecker,
		localProxy:               localProxy,
		autonomyProxy:            autonomyProxy,
		multiplexerProxy:         multiplexerProxy,
		multiplexerManager:       requestMultiplexerManager,
		tenantMgr:                yurtHubCfg.TenantManager,
		nodeName:                 yurtHubCfg.NodeName,
		multiplexerUserAgent:     hubutil.MultiplexerProxyClientUserAgentPrefix + yurtHubCfg.NodeName,
	}

	// warp non resource proxy handler
	return yurtProxy.buildHandlerChain(nonresourcerequest.WrapNonResourceHandler(yurtProxy, yurtHubCfg, cloudHealthChecker)), nil
}

func (p *yurtReverseProxy) buildHandlerChain(handler http.Handler) http.Handler {
	handler = util.WithRequestTrace(handler)
	handler = util.WithRequestContentType(handler)
	handler = util.WithRequestTimeout(handler)
	if !yurtutil.IsNil(p.localProxy) {
		// local cache can not support multiple list requests for same resource from single client,
		// because cache will be overlapped for this client. so we need to use this handler to
		// prevent this case.
		handler = util.WithListRequestSelector(handler)
	}
	handler = util.WithRequestClientComponent(handler)
	handler = util.WithPartialObjectMetadataRequest(handler)
	handler = util.WithIsRequestForPoolScopeMetadata(handler, p.multiplexerManager.IsRequestForPoolScopeMetadata)

	if !yurtutil.IsNil(p.tenantMgr) && p.tenantMgr.GetTenantNs() != "" {
		handler = util.WithSaTokenSubstitute(handler, p.tenantMgr)
	} else {
		klog.V(2).Info("tenant ns is empty, no need to substitute ")
	}

	handler = filters.WithRequestInfo(handler, p.resolver)

	return handler
}

func (p *yurtReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// reject all requests from outside of yurthub when yurthub is not ready.
	// and allow requests from yurthub itself because yurthub need to get resource from cloud kube-apiserver for initializing.
	if !p.IsRequestFromHubSelf(req) {
		if err := config.ReadinessCheck(p.cfg); err != nil {
			klog.Errorf("could not handle request(%s) because hub is not ready for %s", hubutil.ReqString(req), err.Error())
			hubutil.Err(apierrors.NewServiceUnavailable(err.Error()), rw, req)
			return
		}
	}

	// pool scope metadata requests should be handled by multiplexer for both cloud and edge mode.
	isRequestForPoolScopeMetadata, ok := hubutil.IsRequestForPoolScopeMetadataFrom(req.Context())
	if ok && isRequestForPoolScopeMetadata {
		p.multiplexerProxy.ServeHTTP(rw, req)
		return
	}

	switch {
	case yurtutil.IsNil(p.localProxy): // cloud mode
		// requests from local multiplexer of yurthub and the source of pool scope metadata is leader hub
		if p.IsMultiplexerRequestFromHubSelft(req) &&
			p.multiplexerManager.SourceForPoolScopeMetadata() == basemultiplexer.PoolSourceForPoolScopeMetadata {
			// list/watch pool scope metadata from leader yurthub
			if backend := p.loadBalancerForLeaderHub.PickOne(); !yurtutil.IsNil(backend) {
				backend.ServeHTTP(rw, req)
				return
			}
		}

		if backend := p.loadBalancer.PickOne(); !yurtutil.IsNil(backend) {
			backend.ServeHTTP(rw, req)
		} else {
			klog.Errorf("no healthy backend avialbale for request %s", hubutil.ReqString(req))
			http.Error(rw, "no healthy backends available.", http.StatusBadGateway)
		}
	case util.IsKubeletLeaseReq(req):
		p.handleKubeletLease(rw, req)
	case util.IsKubeletGetNodeReq(req):
		p.autonomyProxy.ServeHTTP(rw, req)
	case util.IsEventCreateRequest(req):
		p.eventHandler(rw, req)
	case util.IsSubjectAccessReviewCreateGetRequest(req):
		p.subjectAccessReviewHandler(rw, req)
	case p.IsMultiplexerRequestFromHubSelft(req):
		// requests from multiplexer of local yurthub should be forwarded to cloud kube-apiserver or leader yurthub
		// depends on leader election information.
		if p.multiplexerManager.SourceForPoolScopeMetadata() == basemultiplexer.PoolSourceForPoolScopeMetadata {
			// list/watch pool scope metadata from leader yurthub
			if backend := p.loadBalancerForLeaderHub.PickOne(); !yurtutil.IsNil(backend) {
				backend.ServeHTTP(rw, req)
				return
			}
		}
		// otherwise, list/watch pool scope metadata from cloud kube-apiserver or local cache, so fall through
		fallthrough
	default:
		// handling the request with cloud apiserver or local cache.
		if backend := p.loadBalancer.PickOne(); !yurtutil.IsNil(backend) {
			backend.ServeHTTP(rw, req)
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
	if backend := p.loadBalancer.PickOne(); !yurtutil.IsNil(backend) {
		backend.ServeHTTP(rw, req)
	} else {
		p.localProxy.ServeHTTP(rw, req)
	}
}

func (p *yurtReverseProxy) subjectAccessReviewHandler(rw http.ResponseWriter, req *http.Request) {
	if backend := p.loadBalancer.PickOne(); !yurtutil.IsNil(backend) {
		backend.ServeHTTP(rw, req)
	} else {
		err := errors.New("request is from cloud APIServer but it's currently not healthy")
		klog.Errorf("could not handle SubjectAccessReview req %s, %v", hubutil.ReqString(req), err)
		hubutil.Err(err, rw, req)
	}
}

func (p *yurtReverseProxy) IsRequestFromHubSelf(req *http.Request) bool {
	userAgent := req.UserAgent()

	// yurthub emits the following two kinds of requests
	// 1. requests with User-Agent=multiplexe-proxy-{nodeName} from multiplexer manager in yurthub
	// 2. requests with User-Agent=projectinfo.GetHubName() from sharedInformer for filter and configuration manager in yurthub
	if userAgent == p.multiplexerUserAgent || strings.HasPrefix(userAgent, projectinfo.GetHubName()) {
		return true
	}

	return false
}

func (p *yurtReverseProxy) IsMultiplexerRequestFromHubSelft(req *http.Request) bool {
	userAgent := req.UserAgent()

	if userAgent == p.multiplexerUserAgent {
		return true
	}

	return false
}
