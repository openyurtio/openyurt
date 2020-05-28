package proxy

import (
	"net/http"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/pkg/yurthub/cachemanager"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
	"github.com/alibaba/openyurt/pkg/yurthub/proxy/local"
	"github.com/alibaba/openyurt/pkg/yurthub/proxy/remote"
	"github.com/alibaba/openyurt/pkg/yurthub/proxy/util"
	"github.com/alibaba/openyurt/pkg/yurthub/transport"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/filters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/server"
)

type yurtReverseProxy struct {
	resolver            apirequest.RequestInfoResolver
	loadBalancer        remote.LoadBalancer
	localProxy          *local.LocalProxy
	cacheMgr            cachemanager.CacheManager
	maxRequestsInFlight int
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
		stopCh)
	if err != nil {
		return nil, err
	}

	yurtProxy := &yurtReverseProxy{
		resolver:            resolver,
		loadBalancer:        lb,
		localProxy:          local.NewLocalProxy(cacheMgr, lb.IsHealthy),
		cacheMgr:            cacheMgr,
		maxRequestsInFlight: yurtHubCfg.MaxRequestInFlight,
		stopCh:              stopCh,
	}

	return yurtProxy.buildHandlerChain(yurtProxy), nil
}

func (p *yurtReverseProxy) buildHandlerChain(apiHandler http.Handler) http.Handler {
	handler := util.WithRequestContentType(apiHandler)
	handler = util.WithCacheHeaderCheck(handler)
	handler = util.WithRequestTrace(handler, p.maxRequestsInFlight)
	handler = util.WithRequestClientComponent(handler)
	handler = filters.WithRequestInfo(handler, p.resolver)
	return handler
}

func (p *yurtReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if p.loadBalancer.IsHealthy() {
		p.loadBalancer.ServeHTTP(rw, req)
	} else {
		p.localProxy.ServeHTTP(rw, req)
	}
}
