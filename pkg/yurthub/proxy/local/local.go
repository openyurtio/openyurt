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

package local

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	manager "github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	interval = 2 * time.Second
)

// IsHealthy is func for fetching healthy status of remote server
type IsHealthy func() bool

// LocalProxy is responsible for handling requests when remote servers are unhealthy
type LocalProxy struct {
	cacheMgr  manager.CacheManager
	isHealthy IsHealthy
}

// NewLocalProxy creates a *LocalProxy
func NewLocalProxy(cacheMgr manager.CacheManager, isHealthy IsHealthy) *LocalProxy {
	return &LocalProxy{
		cacheMgr:  cacheMgr,
		isHealthy: isHealthy,
	}
}

// ServeHTTP implements http.Handler for LocalProxy
func (lp *LocalProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error
	ctx := req.Context()
	if req.URL.Path == "/version" {
		err = lp.localServerVersion(w, req)
		klog.V(3).Infof("go into local proxy for request %s", util.ReqString(req))
		if err != nil {
			klog.Errorf("could not proxy local for %s, %v", util.ReqString(req), err)
			util.Err(err, w, req)
		}
		return
	}
	if reqInfo, ok := apirequest.RequestInfoFrom(ctx); ok && reqInfo != nil && reqInfo.IsResourceRequest {
		klog.V(3).Infof("go into local proxy for request %s", util.ReqString(req))
		switch reqInfo.Verb {
		case "watch":
			err = lp.localWatch(w, req)
		case "create":
			err = lp.localPost(w, req)
		case "delete", "deletecollection":
			err = localDelete(w, req)
		default: // list., get, update
			err = lp.localReqCache(w, req)
		}

		if err != nil {
			klog.Errorf("could not proxy local for %s, %v", util.ReqString(req), err)
			util.Err(err, w, req)
		}
	} else {
		klog.Errorf("request(%s) is not supported when cluster is unhealthy", util.ReqString(req))
		util.Err(apierrors.NewBadRequest(fmt.Sprintf("request(%s) is not supported when cluster is unhealthy", util.ReqString(req))), w, req)
	}
}

// localServerVersion handles /version GET requests when remote servers are unhealthy
func (lp *LocalProxy) localServerVersion(w http.ResponseWriter, req *http.Request) error {

	versionInfo, err := lp.cacheMgr.QueryBytes(req)
	copyHeader(w.Header(), req.Header)
	w.WriteHeader(http.StatusOK)
	nw, err := w.Write(versionInfo)
	if err != nil {
		klog.Errorf("write resp for /version request when cluster is unhealthy, expect %d bytes but write %d bytes with error, %v", nw, err)
	}
	klog.V(5).Infof(" get /version info when cluster is unhealthy")
	return err

}

// localDelete handles Delete requests when remote servers are unhealthy
func localDelete(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	s := &metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusForbidden,
		Reason: metav1.StatusReasonForbidden,
		Details: &metav1.StatusDetails{
			Name:  info.Name,
			Group: info.Namespace,
			Kind:  info.Resource,
		},
		Message: "delete request is not supported in local cache",
	}

	util.WriteObject(http.StatusForbidden, s, w, req)
	return nil
}

// localPost handles Create requests when remote servers are unhealthy
func (lp *LocalProxy) localPost(w http.ResponseWriter, req *http.Request) error {
	var buf bytes.Buffer

	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	reqContentType, _ := util.ReqContentTypeFrom(ctx)
	if info.Resource == "events" && len(reqContentType) != 0 {
		ctx = util.WithRespContentType(ctx, reqContentType)
		req = req.WithContext(ctx)
		stopCh := make(chan struct{})
		rc, prc := util.NewDualReadCloser(req, req.Body, false)
		go func(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) {
			klog.V(2).Infof("cache events when cluster is unhealthy, %v", lp.cacheMgr.CacheResponse(req, prc, stopCh))
		}(req, prc, stopCh)

		req.Body = rc
	}

	headerNStr := req.Header.Get("Content-Length")
	headerN, _ := strconv.Atoi(headerNStr)
	n, err := buf.ReadFrom(req.Body)
	if err != nil || (headerN != 0 && int(n) != headerN) {
		klog.Warningf("read body of post request when cluster is unhealthy, expect %d bytes but get %d bytes with error, %v", headerN, n, err)
	}

	// close the pipe only, request body will be closed by http request caller
	if info.Resource == "events" {
		req.Body.Close()
	}

	copyHeader(w.Header(), req.Header)
	w.WriteHeader(http.StatusCreated)

	nw, err := w.Write(buf.Bytes())
	if err != nil || nw != int(n) {
		klog.Errorf("write resp for post request when cluster is unhealthy, expect %d bytes but write %d bytes with error, %v", n, nw, err)
	}
	klog.V(5).Infof("post request %s when cluster is unhealthy", buf.String())

	return nil
}

// localWatch handles Watch requests when remote servers are unhealthy
func (lp *LocalProxy) localWatch(w http.ResponseWriter, req *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		err := fmt.Errorf("unable to start watch - can't get http.Flusher: %#v", w)
		return apierrors.NewInternalError(err)
	}

	opts := metainternalversion.ListOptions{}
	if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err != nil {
		return apierrors.NewBadRequest(err.Error())
	}

	ctx := req.Context()
	contentType, _ := util.ReqContentTypeFrom(ctx)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	timeout := time.Duration(0)
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	} else {
		return nil
	}

	watchTimer := time.NewTimer(timeout)
	intervalTicker := time.NewTicker(interval)
	defer watchTimer.Stop()
	defer intervalTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Infof("exit request %s for context: %v", util.ReqString(req), ctx.Err())
			return nil
		case <-watchTimer.C:
			return nil
		case <-intervalTicker.C:
			// if cluster becomes healthy, exit the watch wait
			if lp.isHealthy() {
				return nil
			}
		}
	}
}

// localReqCache handles Get/List/Update requests when remote servers are unhealthy
func (lp *LocalProxy) localReqCache(w http.ResponseWriter, req *http.Request) error {
	if !lp.cacheMgr.CanCacheFor(req) {
		klog.Errorf("can not cache for %s", util.ReqString(req))
		return apierrors.NewBadRequest(fmt.Sprintf("can not cache for %s", util.ReqString(req)))
	}

	obj, err := lp.cacheMgr.QueryCache(req)
	if errors.Is(err, storage.ErrStorageNotFound) || errors.Is(err, hubmeta.ErrGVRNotRecognized) {
		klog.Errorf("object not found for %s", util.ReqString(req))
		reqInfo, _ := apirequest.RequestInfoFrom(req.Context())
		return apierrors.NewNotFound(schema.GroupResource{Group: reqInfo.APIGroup, Resource: reqInfo.Resource}, reqInfo.Name)
	} else if err != nil {
		klog.Errorf("failed to query cache for %s, %v", util.ReqString(req), err)
		return apierrors.NewInternalError(err)
	} else if obj == nil {
		klog.Errorf("no cache object for %s", util.ReqString(req))
		return apierrors.NewInternalError(fmt.Errorf("no cache object for %s", util.ReqString(req)))
	}

	return util.WriteObject(http.StatusOK, obj, w, req)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if k == "Content-Type" || k == "Content-Length" {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}
