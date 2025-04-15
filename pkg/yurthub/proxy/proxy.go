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
	loadBalancer             remote.Server
	loadBalancerForLeaderHub remote.Server
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
	return yurtProxy.buildHandlerChain(
		nonresourcerequest.WrapNonResourceHandler(yurtProxy, yurtHubCfg, cloudHealthChecker),
	), nil
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
	handler = util.WithRequestForPoolScopeMetadata(handler, p.multiplexerManager.ResolveRequestForPoolScopeMetadata)

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
			klog.Errorf(
				"could not handle request(%s) because hub is not ready for %s",
				hubutil.ReqString(req),
				err.Error(),
			)
			hubutil.Err(apierrors.NewServiceUnavailable(err.Error()), rw, req)
			return
		}
	}

	switch {
	case IsRequestForPoolScopeMetadata(req):
		// requests for list/watching pool scope metadata should be served by following rules:
		// 1. requests from outside of yurthub(like kubelet, kube-proxy, other yurthubs, etc.) should be served by multiplexer proxy.(shouldBeForwarded=false)
		// 2. requests from multiplexer of local yurthub, requests should be forwarded to leader hub or kube-apiserver.(shouldBeForwarded=true)
		// 2.1: if yurthub which emit this request is a follower yurthub, request is forwarded to a leader hub by loadBalancerForLeaderHub(SourceForPoolScopeMetadata()==pool)
		// 2.2: otherwise request is forwarded to kube-apiserver by load balancer.
		shouldBeForwarded, _ := hubutil.ForwardRequestForPoolScopeMetadataFrom(req.Context())
		if shouldBeForwarded {
			// request for pool scope metadata should be forwarded to leader hub or kube-apiserver.
			if p.multiplexerManager.SourceForPoolScopeMetadata() == basemultiplexer.PoolSourceForPoolScopeMetadata {
				// list/watch pool scope metadata from leader yurthub
				if backend := p.loadBalancerForLeaderHub.PickOne(req); !yurtutil.IsNil(backend) {
					backend.ServeHTTP(rw, req)
					return
				}
			}

			// otherwise, list/watch pool scope metadata from cloud kube-apiserver or local cache.
			if backend := p.loadBalancer.PickOne(req); !yurtutil.IsNil(backend) {
				backend.ServeHTTP(rw, req)
				return
			} else if !yurtutil.IsNil(p.localProxy) {
				p.localProxy.ServeHTTP(rw, req)
				return
			}
		} else {
			// request for pool scope metadata should be served by multiplexer manager.
			p.multiplexerProxy.ServeHTTP(rw, req)
			return
		}
		// if the request have not been served, fall into failure serve.
	case util.IsKubeletLeaseReq(req):
		if isServed := p.handleKubeletLease(rw, req); isServed {
			return
		}
		// if the request have not been served, fall into failure serve.
	case util.IsKubeletGetNodeReq(req):
		if isServed := p.handleKubeletGetNode(rw, req); isServed {
			return
		}
		// if the request have not been served, fall into failure serve.
	case util.IsSubjectAccessReviewCreateGetRequest(req):
		if backend := p.loadBalancer.PickOne(req); !yurtutil.IsNil(backend) {
			backend.ServeHTTP(rw, req)
			return
		}
		// if the request have not been served, fall into failure serve.
	default:
		// handling the request with cloud apiserver or local cache, otherwise fail to serve
		if backend := p.loadBalancer.PickOne(req); !yurtutil.IsNil(backend) {
			backend.ServeHTTP(rw, req)
			return
		} else if !yurtutil.IsNil(p.localProxy) {
			p.localProxy.ServeHTTP(rw, req)
			return
		}
		// if the request have not been served, fall into failure serve.
	}

	klog.Errorf("no healthy backend avialbale for request %s", hubutil.ReqString(req))
	http.Error(rw, "no healthy backends available.", http.StatusBadGateway)
}

func (p *yurtReverseProxy) handleKubeletLease(rw http.ResponseWriter, req *http.Request) bool {
	// node lease request should be served by local handler if local proxy is enabled.
	// otherwise, forward node lease request by load balancer.
	isServed := false
	if !yurtutil.IsNil(p.localProxy) {
		p.cloudHealthChecker.RenewKubeletLeaseTime()
		p.localProxy.ServeHTTP(rw, req)
		isServed = true
	} else if backend := p.loadBalancer.PickOne(req); !yurtutil.IsNil(backend) {
		backend.ServeHTTP(rw, req)
		isServed = true
	}
	return isServed
}

func (p *yurtReverseProxy) handleKubeletGetNode(rw http.ResponseWriter, req *http.Request) bool {
	// kubelet get node request should be served by autonomy handler if autonomy proxy is enabled.
	// otherwise, forward kubelet get node request by load balancer.
	isServed := false
	if !yurtutil.IsNil(p.autonomyProxy) {
		p.autonomyProxy.ServeHTTP(rw, req)
		isServed = true
	} else if backend := p.loadBalancer.PickOne(req); !yurtutil.IsNil(backend) {
		backend.ServeHTTP(rw, req)
		isServed = true
	}
	return isServed
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

func IsRequestForPoolScopeMetadata(req *http.Request) bool {
	isRequestForPoolScopeMetadata, ok := hubutil.IsRequestForPoolScopeMetadataFrom(req.Context())
	if ok && isRequestForPoolScopeMetadata {
		return true
	}

	return false
}
