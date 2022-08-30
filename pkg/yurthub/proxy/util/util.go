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

package util

import (
	"context"
	"fmt"
	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"strings"
	"time"

	"gopkg.in/square/go-jose.v2/jwt"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
	"github.com/openyurtio/openyurt/pkg/yurthub/tenant"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	canCacheHeader          string = "Edge-Cache"
	watchTimeoutMargin      int64  = 15
	getAndListTimeoutReduce int64  = 2
)

var needModifyTimeoutVerb = map[string]bool{
	"get":   true,
	"list":  true,
	"watch": true,
}

// WithRequestContentType add req-content-type in request context.
// if no Accept header is set, the request will be reject with a message.
func WithRequestContentType(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				var contentType string
				header := req.Header.Get("Accept")
				parts := strings.Split(header, ",")
				if len(parts) >= 1 {
					contentType = parts[0]
				}

				if len(contentType) != 0 {
					ctx = util.WithReqContentType(ctx, contentType)
					req = req.WithContext(ctx)
				}
			}
		}

		handler.ServeHTTP(w, req)
	})
}

// WithCacheHeaderCheck add cache agent for response cache
// in default mode, only kubelet, kube-proxy, flanneld, coredns User-Agent
// can be supported to cache response. and with Edge-Cache header is also supported.
func WithCacheHeaderCheck(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				needToCache := strings.ToLower(req.Header.Get(canCacheHeader))
				if needToCache == "true" {
					ctx = util.WithReqCanCache(ctx, true)
					req = req.WithContext(ctx)
				}
				req.Header.Del(canCacheHeader)
			}
		}

		handler.ServeHTTP(w, req)
	})
}

// selectorString returns the string of label and field selector
func selectorString(lSelector labels.Selector, fSelector fields.Selector) string {
	var ls string
	var fs string
	if lSelector != nil {
		ls = lSelector.String()
	}

	if fSelector != nil {
		fs = fSelector.String()
	}

	switch {
	case ls != "" && fs != "":
		return strings.Join([]string{ls, fs}, "&")

	case ls != "":
		return ls

	case fs != "":
		return fs
	}

	return ""
}

// WithListRequestSelector add label selector and field selector string in list request context.
func WithListRequestSelector(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest && info.Verb == "list" && info.Name == "" {
				// list request with fieldSelector=metadata.name does not need to set selector string
				opts := metainternalversion.ListOptions{}
				if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err == nil {
					if str := selectorString(opts.LabelSelector, opts.FieldSelector); str != "" {
						ctx = util.WithListSelector(ctx, str)
						req = req.WithContext(ctx)
					}
				}
			}
		}

		handler.ServeHTTP(w, req)
	})
}

// WithRequestClientComponent add component field in request context.
// component is extracted from User-Agent Header, and only the content
// before the "/" when User-Agent include "/".
func WithRequestClientComponent(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {

			if info.IsResourceRequest {
				var comp string
				userAgent := strings.ToLower(req.Header.Get("User-Agent"))
				parts := strings.Split(userAgent, "/")
				if len(parts) > 0 {
					comp = strings.ToLower(parts[0])
				}

				if comp != "" {
					ctx = util.WithClientComponent(ctx, comp)
					req = req.WithContext(ctx)
				}
			}
		}

		handler.ServeHTTP(w, req)
	})
}

type wrapperResponseWriter struct {
	http.ResponseWriter
	http.Flusher
	http.CloseNotifier
	http.Hijacker
	statusCode int
}

func newWrapperResponseWriter(w http.ResponseWriter) *wrapperResponseWriter {
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		klog.Error("can not get http.CloseNotifier")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		klog.Error("can not get http.Flusher")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		klog.Error("can not get http.Hijacker")
	}

	return &wrapperResponseWriter{
		ResponseWriter: w,
		Flusher:        flusher,
		CloseNotifier:  cn,
		Hijacker:       hijacker,
	}
}

func (wrw *wrapperResponseWriter) WriteHeader(statusCode int) {
	wrw.statusCode = statusCode
	wrw.ResponseWriter.WriteHeader(statusCode)
}

// WithRequestTrace used to trace status code and handle time for request.
func WithRequestTrace(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		info, ok := apirequest.RequestInfoFrom(req.Context())
		client, _ := util.ClientComponentFrom(req.Context())
		if ok && info.IsResourceRequest {
			metrics.Metrics.IncInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
			defer metrics.Metrics.DecInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
		}
		wrapperRW := newWrapperResponseWriter(w)
		start := time.Now()
		defer func() {
			klog.Infof("%s with status code %d, spent %v", util.ReqString(req), wrapperRW.statusCode, time.Since(start))
		}()
		handler.ServeHTTP(wrapperRW, req)
	})
}

// WithMaxInFlightLimit limits the number of in-flight requests. and when in flight
// requests exceeds the threshold, the following incoming requests will be rejected.
func WithMaxInFlightLimit(handler http.Handler, limit int) http.Handler {
	var reqChan chan bool
	if limit > 0 {
		reqChan = make(chan bool, limit)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case reqChan <- true:
			klog.V(2).Infof("start proxying: %s %s, in flight requests: %d", strings.ToLower(req.Method), req.URL.String(), len(reqChan))
			defer func() {
				<-reqChan
				klog.V(5).Infof("%s request completed, left %d requests in flight", util.ReqString(req), len(reqChan))
			}()
			handler.ServeHTTP(w, req)
		default:
			// Return a 429 status indicating "Too Many Requests"
			klog.Errorf("Too many requests, please try again later, %s", util.ReqString(req))
			metrics.Metrics.IncRejectedRequestCounter()
			w.Header().Set("Retry-After", "1")
			util.Err(errors.NewTooManyRequestsError("Too many requests, please try again later."), w, req)
		}
	})
}

// 1. WithRequestTimeout add timeout context for watch request.
//    timeout is TimeoutSeconds plus a margin(15 seconds). the timeout
//    context is used to cancel the request for hub missed disconnect
//    signal from kube-apiserver when watch request is ended.
// 2. WithRequestTimeout reduce timeout context for get/list request.
//    timeout is Timout reduce a margin(2 seconds). When request remote server fail,
//    can get data from cache before client timeout.

func WithRequestTimeout(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if info, ok := apirequest.RequestInfoFrom(req.Context()); ok {
			if info.IsResourceRequest && needModifyTimeoutVerb[info.Verb] {
				var timeout time.Duration
				if info.Verb == "list" || info.Verb == "watch" {
					opts := metainternalversion.ListOptions{}
					if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err != nil {
						klog.Errorf("failed to decode parameter for list/watch request: %s", util.ReqString(req))
						util.Err(errors.NewBadRequest(err.Error()), w, req)
						return
					}
					if opts.TimeoutSeconds != nil {
						if info.Verb == "watch" {
							timeout = time.Duration(*opts.TimeoutSeconds+watchTimeoutMargin) * time.Second
						} else if *opts.TimeoutSeconds > getAndListTimeoutReduce {
							timeout = time.Duration(*opts.TimeoutSeconds-getAndListTimeoutReduce) * time.Second
						}
					}
				} else if info.Verb == "get" {
					query := req.URL.Query()
					if str, _ := query["timeout"]; len(str) > 0 {
						if t, err := time.ParseDuration(str[0]); err == nil {
							if t > time.Duration(getAndListTimeoutReduce)*time.Second {
								timeout = t - time.Duration(getAndListTimeoutReduce)*time.Second
							}
						}
					}
				}
				if timeout > 0 {
					ctx, cancel := context.WithTimeout(req.Context(), timeout)
					defer cancel()
					req = req.WithContext(ctx)
				}
			}
		}
		handler.ServeHTTP(w, req)
	})
}

func WithNonResourceRequest(handler http.Handler, cfg *config.YurtHubConfiguration, rest *rest.RestConfigManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/version" {
			cacheNonResourceInfo(w, req, "non-resource-info/version", "/version", cfg, rest)
		} else if req.URL.Path == "/apis/discovery.k8s.io/v1" {
			cacheNonResourceInfo(w, req, "non-resource-info/discoveryV1", "/apis/discovery.k8s.io/v1", cfg, rest)
		} else if req.URL.Path == "/apis/discovery.k8s.io/v1beta1" {
			cacheNonResourceInfo(w, req, "non-resource-info/discoveryV1Beta1", "/apis/discovery.k8s.io/v1beta1", cfg, rest)
		}
		handler.ServeHTTP(w, req)
	})

}

func cacheNonResourceInfo(w http.ResponseWriter, req *http.Request, key string, path string, cfg *config.YurtHubConfiguration, rest *rest.RestConfigManager) {
	infoCache, err := cfg.StorageWrapper.GetRaw(key)
	copyHeader(w.Header(), req.Header)
	if err == nil {
		w.WriteHeader(http.StatusOK)
		klog.Infof("success to query the cache non-resource info: %s", key)
		_, err = w.Write(infoCache)
	} else {
		restCfg := rest.GetRestConfig(false)
		clientSet, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			klog.Errorf("the cluster cannot be connected, the error is: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		} else {
			versionInfo, err := clientSet.RESTClient().Get().AbsPath(path).Do(context.TODO()).Raw()
			if err != nil {
				klog.Errorf("the version info cannot be acquired, the error is: %v", err)
				w.WriteHeader(http.StatusBadRequest)
			}

			klog.Infof("success to non-resource info: %s", key)
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(versionInfo)
			cfg.StorageWrapper.UpdateRaw(key, versionInfo)
		}

	}

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

func WithSaTokenSubstitute(handler http.Handler, tenantMgr tenant.Interface) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {

		if oldToken := util.ParseBearerToken(req.Header.Get("Authorization")); oldToken != "" { //bearer token is not empty&valid

			if jsonWebToken, err := jwt.ParseSigned(oldToken); err != nil {

				klog.Errorf("invaled bearer token %s, err: %v", oldToken, err)
			} else {
				oldClaim := jwt.Claims{}

				if err := jsonWebToken.UnsafeClaimsWithoutVerification(&oldClaim); err == nil {

					if tenantNs, _, err := serviceaccount.SplitUsername(oldClaim.Subject); err == nil {

						if tenantMgr.GetTenantNs() != tenantNs && tenantNs == "kube-system" && tenantMgr.WaitForCacheSync() { // token is not from tenant's namespace
							req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tenantMgr.GetTenantToken()))
							klog.V(2).Infof("replace token, old: %s, new: %s", oldToken, tenantMgr.GetTenantToken())
						}

					} else {
						klog.Errorf("failed to parse tenant ns from token, token %s, sub: %s", oldToken, oldClaim.Subject)
					}
				}
			}

		}

		handler.ServeHTTP(w, req)
	})
}
