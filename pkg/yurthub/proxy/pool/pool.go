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
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator"
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
	isCoordinatorReady   func() bool
	isCloudHealthy       func() bool
}

func NewPoolCoordinatorProxy(
	poolCoordinatorAddr *url.URL,
	localCacheMgr cachemanager.CacheManager,
	coordinator *poolcoordinator.Coordinator,
	transportMgr transport.Interface,
	isCoordinatorReady func() bool,
	isCloudHealthy func() bool,
	stopCh <-chan struct{}) (*PoolCoordinatorProxy, error) {
	if poolCoordinatorAddr == nil {
		return nil, fmt.Errorf("pool-coordinator addr cannot be nil")
	}

	proxy, err := util.NewRemoteProxy(
		poolCoordinatorAddr,
		nil,
		nil,
		transportMgr,
		stopCh)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote proxy for pool-coordinator, %v", err)
	}

	return &PoolCoordinatorProxy{
		poolCoordinatorProxy: proxy,
		localCacheMgr:        localCacheMgr,
		isCoordinatorReady:   isCoordinatorReady,
		isCloudHealthy:       isCloudHealthy,
	}, nil
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
		klog.Errorf("pool-coordinator does not support request(%s) when cluster is unhealthy, requestInfo: %v", hubutil.ReqString(req), reqInfo)
		util.Err(errors.NewBadRequest(fmt.Sprintf("pool-coordinator does not support request(%s) when cluster is unhealthy", hubutil.ReqString(req))), rw, req)
	}
}

func (pp *PoolCoordinatorProxy) poolPost(rw http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	klog.V(4).Infof("pool handle post, req=%s, reqInfo=%s", hubutil.ReqString(req), hubutil.ReqInfoString(info))
	if util.IsSubjectAccessReviewCreateGetRequest(req) || util.IsEventCreateRequest(req) {
		// kubelet needs to create subjectaccessreviews for auth
		pp.poolCoordinatorProxy.ServeHTTP(rw, req)
		return nil
	}

	return fmt.Errorf("unsupported post request")
}

func (pp *PoolCoordinatorProxy) poolQuery(rw http.ResponseWriter, req *http.Request) error {
	if util.IsPoolScopedResouceListWatchRequest(req) || util.IsSubjectAccessReviewCreateGetRequest(req) {
		pp.poolCoordinatorProxy.ServeHTTP(rw, req)
		return nil
	}
	return fmt.Errorf("unsupported query request")
}

func (pp *PoolCoordinatorProxy) poolWatch(rw http.ResponseWriter, req *http.Request) error {
	if util.IsPoolScopedResouceListWatchRequest(req) {
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
