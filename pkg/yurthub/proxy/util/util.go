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
	"net/http"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/munnerz/goautoneg"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
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

// WithRequestForPoolScopeMetadata add marks in context for specifying whether a request is used for list/watching pool scope metadata or not,
// moreover, request for pool scope metadata should be served by multiplexer manager or forwarded to outside.
func WithRequestForPoolScopeMetadata(handler http.Handler, resolveRequestForPoolScopeMetadata func(req *http.Request) (bool, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		isRequestForPoolScopeMetadata, forwardRequestForPoolScopeMetadata := resolveRequestForPoolScopeMetadata(req)
		ctx := req.Context()
		ctx = util.WithIsRequestForPoolScopeMetadata(ctx, isRequestForPoolScopeMetadata)
		ctx = util.WithForwardRequestForPoolScopeMetadata(ctx, forwardRequestForPoolScopeMetadata)

		req = req.WithContext(ctx)
		handler.ServeHTTP(w, req)
	})
}

// WithPartialObjectMetadataRequest is used for extracting info for partial object metadata request,
// then these info is used by cache manager.
func WithPartialObjectMetadataRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				var gvk schema.GroupVersionKind
				acceptHeader := req.Header.Get("Accept")
				clauses := goautoneg.ParseAccept(acceptHeader)
				if len(clauses) >= 1 {
					gvk.Group = clauses[0].Params["g"]
					gvk.Version = clauses[0].Params["v"]
					gvk.Kind = clauses[0].Params["as"]
				}

				if gvk.Kind == "PartialObjectMetadataList" || gvk.Kind == "PartialObjectMetadata" {
					if err := ensureValidGroupAndVersion(&gvk); err != nil {
						klog.Errorf("WithPartialObjectMetadataRequest error: %v", err)
					} else {
						ctx = util.WithConvertGVK(ctx, &gvk)
						req = req.WithContext(ctx)
					}
				}
			}
		}

		handler.ServeHTTP(w, req)
	})
}

func ensureValidGroupAndVersion(gvk *schema.GroupVersionKind) error {
	if strings.Contains(gvk.Group, "meta.k8s.io") {
		gvk.Group = "meta.k8s.io"
	} else {
		return fmt.Errorf("unknown group(%s) for partialobjectmetadata request", gvk.Group)
	}

	switch {
	case strings.Contains(gvk.Version, "v1"):
		gvk.Version = "v1"
	case strings.Contains(gvk.Version, "v1beta1"):
		gvk.Version = "v1beta1"
	default:
		return fmt.Errorf("unknown version(%s) for partialobjectmetadata request", gvk.Version)
	}

	return nil
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

				subParts := strings.Split(contentType, ";")
				for i := range subParts {
					if strings.Contains(subParts[i], "as=") {
						contentType = subParts[0]
						break
					}
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

// WithRequestClientComponent adds user agent header in request context.
func WithRequestClientComponent(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				userAgent := strings.ToLower(req.Header.Get("User-Agent"))
				if userAgent != "" {
					ctx = util.WithClientComponent(ctx, userAgent)
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
	http.Hijacker
	statusCode int
}

func newWrapperResponseWriter(w http.ResponseWriter) *wrapperResponseWriter {
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
		Hijacker:       hijacker,
	}
}

func (wrw *wrapperResponseWriter) WriteHeader(statusCode int) {
	wrw.statusCode = statusCode
	wrw.ResponseWriter.WriteHeader(statusCode)
}

// WithRequestTrace used to trace status code and request metrics.
// at the same time, print detailed logs of request at start and end serving point.
func WithRequestTrace(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		client, _ := util.ClientComponentFrom(ctx)
		isRequestForPoolScopeMetadata, _ := util.IsRequestForPoolScopeMetadataFrom(ctx)
		shouldBeForward, _ := util.ForwardRequestForPoolScopeMetadataFrom(ctx)
		info, ok := apirequest.RequestInfoFrom(ctx)
		if !ok {
			info = &apirequest.RequestInfo{}
		}

		// inc metrics for recording request
		if info.IsResourceRequest {
			if isRequestForPoolScopeMetadata && !shouldBeForward {
				metrics.Metrics.IncAggregatedInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
			} else {
				metrics.Metrics.IncInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
			}
		}

		// print logs at start serving point
		if info.Resource == "leases" {
			klog.V(5).Infof("%s request is going to be served", util.ReqString(req))
		} else if isRequestForPoolScopeMetadata {
			klog.V(2).Infof("%s request for pool scope metadata is going to be served", util.ReqString(req))
		} else {
			klog.V(2).Infof("%s request is going to be served", util.ReqString(req))
		}

		wrapperRW := newWrapperResponseWriter(w)
		start := time.Now()
		defer func() {
			duration := time.Since(start)
			// dec metrics for recording request
			if info.IsResourceRequest {
				if isRequestForPoolScopeMetadata && !shouldBeForward {
					metrics.Metrics.DecAggregatedInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
				} else {
					metrics.Metrics.DecInFlightRequests(info.Verb, info.Resource, info.Subresource, client)
				}
			}

			// print logs at end serving point
			if info.Resource == "leases" {
				klog.V(5).Infof("%s request with status code %d, spent %v", util.ReqString(req), wrapperRW.statusCode, duration)
			} else if isRequestForPoolScopeMetadata {
				klog.V(2).Infof("%s request for pool scope metadata with status code %d, spent %v", util.ReqString(req), wrapperRW.statusCode, duration)
			} else {
				klog.V(2).Infof("%s request with status code %d, spent %v", util.ReqString(req), wrapperRW.statusCode, duration)
			}
		}()
		handler.ServeHTTP(wrapperRW, req)
	})
}

//  1. WithRequestTimeout add timeout context for watch request.
//     timeout is TimeoutSeconds plus a margin(15 seconds). the timeout
//     context is used to cancel the request for hub missed disconnect
//     signal from kube-apiserver when watch request is ended.
//  2. WithRequestTimeout reduce timeout context for get/list request.
//     timeout is Timeout reduce a margin(2 seconds). When request remote server fail,
//     can get data from cache before client timeout.
func WithRequestTimeout(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if info, ok := apirequest.RequestInfoFrom(req.Context()); ok {
			if info.IsResourceRequest && needModifyTimeoutVerb[info.Verb] {
				var timeout time.Duration
				if info.Verb == "list" || info.Verb == "watch" {
					opts := metainternalversion.ListOptions{}
					if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err != nil {
						klog.Errorf("could not decode parameter for list/watch request: %s", util.ReqString(req))
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
					if str := query["timeout"]; len(str) > 0 {
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

func WithSaTokenSubstitute(handler http.Handler, tenantMgr tenant.Interface) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {

		if oldToken := util.ParseBearerToken(req.Header.Get("Authorization")); oldToken != "" { // bearer token is not empty&valid

			if jsonWebToken, err := jwt.ParseSigned(oldToken); err != nil {

				klog.Errorf("invalid bearer token %s, err: %v", oldToken, err)
			} else {
				oldClaim := jwt.Claims{}

				if err := jsonWebToken.UnsafeClaimsWithoutVerification(&oldClaim); err == nil {

					if tenantNs, _, err := serviceaccount.SplitUsername(oldClaim.Subject); err == nil {

						if tenantMgr.GetTenantNs() != tenantNs && tenantNs == "kube-system" && tenantMgr.WaitForCacheSync(req.Context()) { // token is not from tenant's namespace
							req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tenantMgr.GetTenantToken()))
							klog.V(2).Infof("replace token, old: %s, new: %s", oldToken, tenantMgr.GetTenantToken())
						}

					} else {
						klog.Errorf("could not parse tenant ns from token, token %s, sub: %s", oldToken, oldClaim.Subject)
					}
				}
			}

		}

		handler.ServeHTTP(w, req)
	})
}

func WithObjectFilter(handler http.Handler, filterFinder filter.FilterFinder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		objectFilter, ok := filterFinder.FindObjectFilter(req)
		if ok {
			ctx := util.WithObjectFilter(req.Context(), objectFilter)
			req = req.WithContext(ctx)
		}

		handler.ServeHTTP(w, req)
	})
}

// IsListRequestWithNameFieldSelector will check if the request has FieldSelector "metadata.name".
// If found, return true, otherwise false.
func IsListRequestWithNameFieldSelector(req *http.Request) bool {
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		if info.IsResourceRequest && info.Verb == "list" {
			opts := metainternalversion.ListOptions{}
			if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err == nil {
				if opts.FieldSelector == nil {
					return false
				}
				if _, found := opts.FieldSelector.RequiresExactMatch("metadata.name"); found {
					return true
				}
			}
		}
	}
	return false
}

// IsKubeletLeaseReq judge whether the request is a lease request from kubelet
func IsKubeletLeaseReq(req *http.Request) bool {
	ctx := req.Context()
	if comp, ok := util.TruncatedClientComponentFrom(ctx); !ok || comp != "kubelet" {
		return false
	}
	if info, ok := apirequest.RequestInfoFrom(ctx); !ok || info.Resource != "leases" {
		return false
	}
	return true
}

// IsKubeletGetNodeReq judge whether the request is a get node request from kubelet
func IsKubeletGetNodeReq(req *http.Request) bool {
	ctx := req.Context()
	if comp, ok := util.TruncatedClientComponentFrom(ctx); !ok || comp != "kubelet" {
		return false
	}
	if info, ok := apirequest.RequestInfoFrom(ctx); !ok || info.Resource != "nodes" || info.Verb != "get" {
		return false
	}
	return true
}

func IsSubjectAccessReviewCreateGetRequest(req *http.Request) bool {
	ctx := req.Context()
	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok {
		return false
	}

	comp, ok := util.TruncatedClientComponentFrom(ctx)
	if !ok {
		return false
	}

	return info.IsResourceRequest &&
		comp == "kubelet" &&
		info.Resource == "subjectaccessreviews" &&
		(info.Verb == "create" || info.Verb == "get")
}
