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

package util

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
)

// ProxyKeyType represents the key in proxy request context
type ProxyKeyType int

// WorkingMode represents the working mode of yurthub.
type WorkingMode string

const (
	// WorkingModeCloud represents yurthub is working in cloud mode, which means yurthub is deployed on the cloud side.
	WorkingModeCloud WorkingMode = "cloud"
	// WorkingModeEdge represents yurthub is working in edge mode, which means yurthub is deployed on the edge side.
	WorkingModeEdge WorkingMode = "edge"
	// WorkingModeLocal represents yurthub is working in local mode, which means yurthub is deployed on the local side.
	WorkingModeLocal WorkingMode = "local"

	// ProxyReqContentType represents request content type context key
	ProxyReqContentType ProxyKeyType = iota
	// ProxyRespContentType represents response content type context key
	ProxyRespContentType
	// ProxyClientComponent represents client component context key
	ProxyClientComponent
	// ProxyReqCanCache represents request can cache context key
	ProxyReqCanCache
	// ProxyListSelector represents label selector and filed selector string for list request
	ProxyListSelector
	// ProxyPartialObjectMetadataRequest represents if this request is getting partial object metadata
	ProxyPartialObjectMetadataRequest
	// ProxyConvertGVK represents the gvk of response when it is a partial object metadata request
	ProxyConvertGVK

	YurtHubNamespace      = "kube-system"
	CacheUserAgentsKey    = "cache_agents"
	PoolScopeResourcesKey = "pool_scope_resources"

	MultiplexerProxyClientUserAgentPrefix = "multiplexer-proxy-"

	YurtHubProxyPort       = 10261
	YurtHubPort            = 10267
	YurtHubProxySecurePort = 10268
)

var (
	YurthubConfigMapName = fmt.Sprintf("%s-hub-cfg", strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
)

// WithValue returns a copy of parent in which the value associated with key is val.
func WithValue(parent context.Context, key interface{}, val interface{}) context.Context {
	return context.WithValue(parent, key, val)
}

// WithReqContentType returns a copy of parent in which the response content type value is set
func WithReqContentType(parent context.Context, contentType string) context.Context {
	return WithValue(parent, ProxyReqContentType, contentType)
}

// ReqContentTypeFrom returns the value of the request content type key on the ctx
func ReqContentTypeFrom(ctx context.Context) (string, bool) {
	info, ok := ctx.Value(ProxyReqContentType).(string)
	return info, ok
}

// WithRespContentType returns a copy of parent in which the request content type value is set
func WithRespContentType(parent context.Context, contentType string) context.Context {
	return WithValue(parent, ProxyRespContentType, contentType)
}

// RespContentTypeFrom returns the value of the response content type key on the ctx
func RespContentTypeFrom(ctx context.Context) (string, bool) {
	info, ok := ctx.Value(ProxyRespContentType).(string)
	return info, ok
}

// WithClientComponent returns a copy of parent in which the client component value is set
func WithClientComponent(parent context.Context, component string) context.Context {
	return WithValue(parent, ProxyClientComponent, component)
}

// ClientComponentFrom returns the value of the client component key on the ctx
func ClientComponentFrom(ctx context.Context) (string, bool) {
	info, ok := ctx.Value(ProxyClientComponent).(string)
	return info, ok
}

// WithReqCanCache returns a copy of parent in which the request can cache value is set
func WithReqCanCache(parent context.Context, canCache bool) context.Context {
	return WithValue(parent, ProxyReqCanCache, canCache)
}

// ReqCanCacheFrom returns the value of the request can cache key on the ctx
func ReqCanCacheFrom(ctx context.Context) (bool, bool) {
	info, ok := ctx.Value(ProxyReqCanCache).(bool)
	return info, ok
}

// WithListSelector returns a copy of parent in which the list request selector string is set
func WithListSelector(parent context.Context, selector string) context.Context {
	return WithValue(parent, ProxyListSelector, selector)
}

// ListSelectorFrom returns the value of the list request selector string on the ctx
func ListSelectorFrom(ctx context.Context) (string, bool) {
	info, ok := ctx.Value(ProxyListSelector).(string)
	return info, ok
}

// WithConvertGVK returns a copy of parent in which the convert gvk value is set
func WithConvertGVK(parent context.Context, gvk *schema.GroupVersionKind) context.Context {
	return WithValue(parent, ProxyConvertGVK, gvk)
}

// ConvertGVKFrom returns the value of the convert gvk key on the ctx
func ConvertGVKFrom(ctx context.Context) (*schema.GroupVersionKind, bool) {
	info, ok := ctx.Value(ProxyConvertGVK).(*schema.GroupVersionKind)
	return info, ok
}

// ReqString formats a string for request
func ReqString(req *http.Request) string {
	ctx := req.Context()
	comp, _ := ClientComponentFrom(ctx)
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		return fmt.Sprintf("%v %s %s: %s", comp, info.Verb, info.Resource, req.URL.String())
	}

	return fmt.Sprintf("%s of %s", comp, req.URL.String())
}

// ReqInfoString formats a string for request info
func ReqInfoString(info *apirequest.RequestInfo) string {
	if info == nil {
		return ""
	}

	return fmt.Sprintf("%s %s for %s", info.Verb, info.Resource, info.Path)
}

// Err write err to response writer
func Err(err error, w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		gv := schema.GroupVersion{
			Group:   info.APIGroup,
			Version: info.APIVersion,
		}
		negotiatedSerializer := serializer.YurtHubSerializer.GetNegotiatedSerializer(gv.WithResource(info.Resource))
		responsewriters.ErrorNegotiated(err, negotiatedSerializer, gv, w, req)
		return
	}

	klog.Errorf("request info is not found when err write, %s", ReqString(req))
}

// WriteObject write object to response writer
func WriteObject(statusCode int, obj runtime.Object, w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		gvr := schema.GroupVersionResource{
			Group:    info.APIGroup,
			Version:  info.APIVersion,
			Resource: info.Resource,
		}

		convertGVK, ok := ConvertGVKFrom(ctx)
		if ok && convertGVK != nil {
			gvr, _ = meta.UnsafeGuessKindToResource(*convertGVK)
		}

		negotiatedSerializer := serializer.YurtHubSerializer.GetNegotiatedSerializer(gvr)
		responsewriters.WriteObjectNegotiated(negotiatedSerializer, DefaultHubEndpointRestrictions, gvr.GroupVersion(), w, req, statusCode, obj, false)
		return nil
	}

	return fmt.Errorf("request info is not found when write object, %s", ReqString(req))
}

// DefaultHubEndpointRestrictions is the default EndpointRestrictions which allows
// content-type negotiation to verify yurthub server support for specific options
var DefaultHubEndpointRestrictions = hubEndpointRestrictions{}

type hubEndpointRestrictions struct{}

func (hubEndpointRestrictions) AllowsMediaTypeTransform(mimeType string, mimeSubType string, gvk *schema.GroupVersionKind) bool {
	if gvk == nil {
		return true
	}

	if gvk.GroupVersion() == metav1beta1.SchemeGroupVersion || gvk.GroupVersion() == metav1.SchemeGroupVersion {
		switch gvk.Kind {
		case "PartialObjectMetadata", "PartialObjectMetadataList":
			return true
		default:
			return false
		}
	}
	return false
}
func (hubEndpointRestrictions) AllowsServerVersion(string) bool  { return false }
func (hubEndpointRestrictions) AllowsStreamSchema(s string) bool { return s == "watch" }

// NewDualReadCloser create an dualReadCloser object
func NewDualReadCloser(req *http.Request, rc io.ReadCloser, isRespBody bool) (io.ReadCloser, io.ReadCloser) {
	pr, pw := io.Pipe()
	dr := &dualReadCloser{
		rc:         rc,
		pw:         pw,
		isRespBody: isRespBody,
	}

	return dr, pr
}

type dualReadCloser struct {
	rc io.ReadCloser
	pw *io.PipeWriter
	// isRespBody shows rc(is.ReadCloser) is a response.Body
	// or not(maybe a request.Body). if it is true(it's a response.Body),
	// we should close the response body in Close func, else not,
	// it(request body) will be closed by http request caller
	isRespBody bool
}

// Read read data into p and write into pipe
func (dr *dualReadCloser) Read(p []byte) (n int, err error) {
	n, err = dr.rc.Read(p)
	if n > 0 {
		if n, err := dr.pw.Write(p[:n]); err != nil {
			klog.Errorf("dualReader: could not write %v", err)
			return n, err
		}
	}

	return
}

// Close close two readers
func (dr *dualReadCloser) Close() error {
	errs := make([]error, 0)
	if dr.isRespBody {
		if err := dr.rc.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if err := dr.pw.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return fmt.Errorf("could not close dualReader, %v", errs)
	}

	return nil
}

// SplitKey split key into comp, resource, ns, name
func SplitKey(key string) (comp, resource, ns, name string) {
	if len(key) == 0 {
		return
	}

	parts := strings.Split(key, "/")
	switch len(parts) {
	case 1:
		comp = parts[0]
	case 2:
		comp = parts[0]
		resource = parts[1]
	case 3:
		comp = parts[0]
		resource = parts[1]
		name = parts[2]
	case 4:
		comp = parts[0]
		resource = parts[1]
		ns = parts[2]
		name = parts[3]
	}

	return
}

// IsSupportedLBMode check lb mode is supported or not
func IsSupportedLBMode(lbMode string) bool {
	switch lbMode {
	case "rr", "priority":
		return true
	}

	return false
}

// IsSupportedWorkingMode check working mode is supported or not
func IsSupportedWorkingMode(workingMode WorkingMode) bool {
	switch workingMode {
	case WorkingModeCloud, WorkingModeEdge, WorkingModeLocal:
		return true
	}

	return false
}

// FileExists checks if specified file exists.
func FileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// gzipReaderCloser will gunzip the data if response header
// contains Content-Encoding=gzip header.
type gzipReaderCloser struct {
	body io.ReadCloser
	zr   *gzip.Reader
	zerr error
}

func (grc *gzipReaderCloser) Read(b []byte) (n int, err error) {
	if grc.zerr != nil {
		return 0, grc.zerr
	}

	if grc.zr == nil {
		grc.zr, err = gzip.NewReader(grc.body)
		if err != nil {
			grc.zerr = err
			return 0, err
		}
	}

	return grc.zr.Read(b)
}

func (grc *gzipReaderCloser) Close() error {
	return grc.body.Close()
}

func NewGZipReaderCloser(header http.Header, body io.ReadCloser, req *http.Request, caller string) (io.ReadCloser, bool) {
	if header.Get("Content-Encoding") != "gzip" {
		return body, false
	}

	klog.Infof("response of %s will be ungzip at %s", ReqString(req), caller)
	return &gzipReaderCloser{
		body: body,
	}, true
}

func ParseTenantNs(certOrg string) string {
	if !strings.Contains(certOrg, "openyurt:tenant:") {
		return ""
	}

	return strings.TrimPrefix(certOrg, "openyurt:tenant:")
}

func ParseTenantNsFromOrgs(orgs []string) string {
	var ns string
	if len(orgs) == 0 {
		return ns
	}

	for _, v := range orgs {
		ns := ParseTenantNs(v)
		if len(ns) != 0 {
			return ns
		}
	}

	return ns
}

func ParseBearerToken(token string) string {
	if token == "" {
		return ""
	}

	if !strings.HasPrefix(token, "Bearer ") { //not invalid bearer token
		return ""
	}

	return strings.TrimPrefix(token, "Bearer ")
}

type TrafficTraceReader struct {
	rc          io.ReadCloser // original response body
	client      string
	verb        string
	resource    string
	subResource string
}

// Read overwrite Read function of io.ReadCloser in order to trace traffic for each request
func (tt *TrafficTraceReader) Read(p []byte) (n int, err error) {
	n, err = tt.rc.Read(p)
	metrics.Metrics.AddProxyTrafficCollector(tt.client, tt.verb, tt.resource, tt.subResource, n)
	return
}

func (tt *TrafficTraceReader) Close() error {
	return tt.rc.Close()
}

func WrapWithTrafficTrace(req *http.Request, resp *http.Response) *http.Response {
	ctx := req.Context()
	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || !info.IsResourceRequest {
		return resp
	}
	comp, ok := ClientComponentFrom(ctx)
	if !ok || len(comp) == 0 {
		return resp
	}

	resp.Body = &TrafficTraceReader{
		rc:          resp.Body,
		client:      comp,
		verb:        info.Verb,
		resource:    info.Resource,
		subResource: info.Subresource,
	}
	return resp
}

func FromApiserverCache(opts *metav1.GetOptions) {
	opts.ResourceVersion = "0"
}

func NodeConditionsHaveChanged(originalConditions []v1.NodeCondition, conditions []v1.NodeCondition) bool {
	if len(originalConditions) != len(conditions) {
		return true
	}

	originalConditionsCopy := make([]v1.NodeCondition, 0, len(originalConditions))
	originalConditionsCopy = append(originalConditionsCopy, originalConditions...)
	conditionsCopy := make([]v1.NodeCondition, 0, len(conditions))
	conditionsCopy = append(conditionsCopy, conditions...)

	sort.SliceStable(originalConditionsCopy, func(i, j int) bool { return originalConditionsCopy[i].Type < originalConditionsCopy[j].Type })
	sort.SliceStable(conditionsCopy, func(i, j int) bool { return conditionsCopy[i].Type < conditionsCopy[j].Type })

	replacedheartbeatTime := metav1.Time{}
	for i := range conditionsCopy {
		originalConditionsCopy[i].LastHeartbeatTime = replacedheartbeatTime
		conditionsCopy[i].LastHeartbeatTime = replacedheartbeatTime
		if !apiequality.Semantic.DeepEqual(&originalConditionsCopy[i], &conditionsCopy[i]) {
			return true
		}
	}
	return false
}
