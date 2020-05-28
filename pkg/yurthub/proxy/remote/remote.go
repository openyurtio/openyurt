package remote

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/alibaba/openyurt/pkg/yurthub/cachemanager"
	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
	"github.com/alibaba/openyurt/pkg/yurthub/transport"
	"github.com/alibaba/openyurt/pkg/yurthub/util"

	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog"
)

// RemoteProxy is an reverse proxy for remote server
type RemoteProxy struct {
	checker      healthchecker.HealthChecker
	reverseProxy *httputil.ReverseProxy
	cacheMgr     cachemanager.CacheManager
	remoteServer *url.URL
	stopCh       <-chan struct{}
}

// NewRemoteProxy creates an *RemoteProxy object, and will be used by LoadBalancer
func NewRemoteProxy(remoteServer *url.URL,
	cacheMgr cachemanager.CacheManager,
	transportMgr transport.Interface,
	healthChecker healthchecker.HealthChecker,
	stopCh <-chan struct{}) (*RemoteProxy, error) {
	currentTransport := transportMgr.CurrentTransport()
	if currentTransport == nil {
		return nil, fmt.Errorf("could not get current transport when init proxy backend(%s)", remoteServer.String())
	}

	proxyBackend := &RemoteProxy{
		checker:      healthChecker,
		reverseProxy: httputil.NewSingleHostReverseProxy(remoteServer),
		cacheMgr:     cacheMgr,
		remoteServer: remoteServer,
		stopCh:       stopCh,
	}

	proxyBackend.reverseProxy.Transport = currentTransport
	proxyBackend.reverseProxy.ModifyResponse = proxyBackend.modifyResponse
	proxyBackend.reverseProxy.FlushInterval = -1

	return proxyBackend, nil
}

// Name represents the address of remote server
func (rp *RemoteProxy) Name() string {
	return rp.remoteServer.String()
}

func (rp *RemoteProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rp.reverseProxy.ServeHTTP(rw, req)
}

// IsHealthy returns healthy status of remote server
func (rp *RemoteProxy) IsHealthy() bool {
	return rp.checker.IsHealthy(rp.remoteServer)
}

func (rp *RemoteProxy) modifyResponse(resp *http.Response) error {
	if resp == nil || resp.Request == nil {
		klog.Infof("no request info in response, skip cache response")
		return nil
	}

	req := resp.Request
	ctx := req.Context()

	// re-added transfer-encoding=chunked response header for watch request
	if info, exists := apirequest.RequestInfoFrom(ctx); exists {
		if info.Verb == "watch" {
			klog.V(5).Infof("add transfer-encoding=chunked header into response for req %s", util.ReqString(req))
			h := resp.Header
			if hv := h.Get("Transfer-Encoding"); hv == "" {
				h.Add("Transfer-Encoding", "chunked")
			}
		}
	}

	// cache resp with storage interface
	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusPartialContent {
		if rp.cacheMgr.CanCacheFor(req) {
			respContentType := resp.Header.Get("Content-Type")
			ctx = util.WithRespContentType(ctx, respContentType)
			reqContentType, _ := util.ReqContentTypeFrom(ctx)
			if len(reqContentType) == 0 || reqContentType == "*/*" {
				ctx = util.WithReqContentType(ctx, respContentType)
			}

			rc, prc := util.NewDualReadCloser(resp.Body, true)
			go func(ctx context.Context, prc io.ReadCloser, stopCh <-chan struct{}) {
				err := rp.cacheMgr.CacheResponse(ctx, prc, stopCh)
				if err != nil && err != io.EOF && err != context.Canceled {
					klog.Errorf("%s response cache ended with error, %v", util.ReqString(req), err)
				}
			}(ctx, prc, rp.stopCh)

			resp.Body = rc
		}
	}
	return nil
}
