/*
Copyright 2022 The OpenYurt Authors.

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

package pool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/manager"
	"github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	hubutil "github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	watchCheckInterval = 5 * time.Second
)

// PoolCoordinatorProxy is responsible for handling requests when remote servers are unhealthy
type PoolCoordinatorProxy struct {
	poolCoordinatorProxy *util.RemoteProxy
	localCacheMgr        cachemanager.CacheManager
	filterMgr            *manager.Manager
	isCoordinatorReady   func() bool
	stopCh               <-chan struct{}
}

func NewPoolCoordinatorProxy(
	poolCoordinatorAddr *url.URL,
	localCacheMgr cachemanager.CacheManager,
	transportMgrGetter func() transport.Interface,
	filterMgr *manager.Manager,
	isCoordinatorReady func() bool,
	stopCh <-chan struct{}) (*PoolCoordinatorProxy, error) {
	if poolCoordinatorAddr == nil {
		return nil, fmt.Errorf("pool-coordinator addr cannot be nil")
	}

	pp := &PoolCoordinatorProxy{
		localCacheMgr:      localCacheMgr,
		isCoordinatorReady: isCoordinatorReady,
		filterMgr:          filterMgr,
		stopCh:             stopCh,
	}

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-ticker.C:
				transportMgr := transportMgrGetter()
				if transportMgr == nil {
					break
				}
				proxy, err := util.NewRemoteProxy(
					poolCoordinatorAddr,
					pp.modifyResponse,
					pp.errorHandler,
					transportMgr,
					stopCh)
				if err != nil {
					klog.Errorf("failed to create remote proxy for pool-coordinator, %v", err)
					return
				}

				pp.poolCoordinatorProxy = proxy
				klog.Infof("create remote proxy for pool-coordinator success")
				return
			}
		}
	}()

	return pp, nil
}

// ServeHTTP of PoolCoordinatorProxy is able to handle read-only request, including
// watch, list, get. Other verbs that will write data to the cache are not supported
// currently.
func (pp *PoolCoordinatorProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var err error
	ctx := req.Context()
	reqInfo, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || reqInfo == nil {
		klog.Errorf("pool-coordinator proxy cannot handle request(%s), cannot get requestInfo", hubutil.ReqString(req), reqInfo)
		util.Err(errors.NewBadRequest(fmt.Sprintf("pool-coordinator proxy cannot handle request(%s), cannot get requestInfo", hubutil.ReqString(req))), rw, req)
		return
	}
	req.Header.Del("Authorization") // delete token with cloud apiServer RBAC and use yurthub authorization
	if reqInfo.IsResourceRequest {
		switch reqInfo.Verb {
		case "create":
			err = pp.poolPost(rw, req)
		case "list", "get":
			err = pp.poolQuery(rw, req)
		case "watch":
			err = pp.poolWatch(rw, req)
		default:
			err = fmt.Errorf("unsupported verb for pool coordinator proxy: %s", reqInfo.Verb)
		}
		if err != nil {
			klog.Errorf("could not proxy to pool-coordinator for %s, %v", hubutil.ReqString(req), err)
			util.Err(errors.NewBadRequest(err.Error()), rw, req)
		}
	} else {
		klog.Errorf("pool-coordinator does not support request(%s), requestInfo: %s", hubutil.ReqString(req), hubutil.ReqInfoString(reqInfo))
		util.Err(errors.NewBadRequest(fmt.Sprintf("pool-coordinator does not support request(%s)", hubutil.ReqString(req))), rw, req)
	}
}

func (pp *PoolCoordinatorProxy) poolPost(rw http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	klog.V(4).Infof("pool handle post, req=%s, reqInfo=%s", hubutil.ReqString(req), hubutil.ReqInfoString(info))
	if (util.IsSubjectAccessReviewCreateGetRequest(req) || util.IsEventCreateRequest(req)) && pp.poolCoordinatorProxy != nil {
		// kubelet needs to create subjectaccessreviews for auth
		pp.poolCoordinatorProxy.ServeHTTP(rw, req)
		return nil
	}

	return fmt.Errorf("unsupported post request")
}

func (pp *PoolCoordinatorProxy) poolQuery(rw http.ResponseWriter, req *http.Request) error {
	if (util.IsPoolScopedResouceListWatchRequest(req) || util.IsSubjectAccessReviewCreateGetRequest(req)) && pp.poolCoordinatorProxy != nil {
		pp.poolCoordinatorProxy.ServeHTTP(rw, req)
		return nil
	}
	return fmt.Errorf("unsupported query request")
}

func (pp *PoolCoordinatorProxy) poolWatch(rw http.ResponseWriter, req *http.Request) error {
	if util.IsPoolScopedResouceListWatchRequest(req) && pp.poolCoordinatorProxy != nil {
		clientReqCtx := req.Context()
		poolServeCtx, poolServeCancel := context.WithCancel(clientReqCtx)

		go func() {
			t := time.NewTicker(watchCheckInterval)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					if !pp.isCoordinatorReady() {
						klog.Infof("notified the pool coordinator is not ready for handling request, cancel watch %s", hubutil.ReqString(req))
						poolServeCancel()
						return
					}
				case <-clientReqCtx.Done():
					klog.Infof("notified client canceled the watch request %s, stop proxy it to pool coordinator", hubutil.ReqString(req))
					return
				}
			}
		}()

		newReq := req.Clone(poolServeCtx)
		pp.poolCoordinatorProxy.ServeHTTP(rw, newReq)
		klog.Infof("watch %s to pool coordinator exited", hubutil.ReqString(req))
		return nil
	}
	return fmt.Errorf("unsupported watch request")
}

func (pp *PoolCoordinatorProxy) errorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	klog.Errorf("remote proxy error handler: %s, %v", hubutil.ReqString(req), err)
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		if info.Verb == "get" || info.Verb == "list" {
			if obj, err := pp.localCacheMgr.QueryCache(req); err == nil {
				hubutil.WriteObject(http.StatusOK, obj, rw, req)
				return
			}
		}
	}
	rw.WriteHeader(http.StatusBadGateway)
}

func (pp *PoolCoordinatorProxy) modifyResponse(resp *http.Response) error {
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

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusPartialContent {
		// prepare response content type
		reqContentType, _ := hubutil.ReqContentTypeFrom(ctx)
		respContentType := resp.Header.Get("Content-Type")
		if len(respContentType) == 0 {
			respContentType = reqContentType
		}
		ctx = hubutil.WithRespContentType(ctx, respContentType)
		req = req.WithContext(ctx)

		// filter response data
		if pp.filterMgr != nil {
			if responseFilter, ok := pp.filterMgr.FindResponseFilter(req); ok {
				wrapBody, needUncompressed := hubutil.NewGZipReaderCloser(resp.Header, resp.Body, req, "filter")
				size, filterRc, err := responseFilter.Filter(req, wrapBody, pp.stopCh)
				if err != nil {
					klog.Errorf("failed to filter response for %s, %v", hubutil.ReqString(req), err)
					return err
				}
				resp.Body = filterRc
				if size > 0 {
					resp.ContentLength = int64(size)
					resp.Header.Set("Content-Length", fmt.Sprint(size))
				}

				// after gunzip in filter, the header content encoding should be removed.
				// because there's no need to gunzip response.body again.
				if needUncompressed {
					resp.Header.Del("Content-Encoding")
				}
			}
		}
		// cache resp with storage interface
		pp.cacheResponse(req, resp)
	}

	return nil
}

func (pp *PoolCoordinatorProxy) cacheResponse(req *http.Request, resp *http.Response) {
	if pp.localCacheMgr.CanCacheFor(req) {
		ctx := req.Context()
		req = req.WithContext(ctx)
		wrapPrc, needUncompressed := hubutil.NewGZipReaderCloser(resp.Header, resp.Body, req, "cache-manager")

		rc, prc := hubutil.NewDualReadCloser(req, wrapPrc, true)
		go func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) {
			if err := pp.localCacheMgr.CacheResponse(req, prc, stopCh); err != nil {
				klog.Errorf("pool proxy failed to cache req %s in local cache, %v", hubutil.ReqString(req), err)
			}
		}(req, prc, ctx.Done())

		// after gunzip in filter, the header content encoding should be removed.
		// because there's no need to gunzip response.body again.
		if needUncompressed {
			resp.Header.Del("Content-Encoding")
		}
		resp.Body = rc
	}
}
