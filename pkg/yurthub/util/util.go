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
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
	coordinatorconstants "github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/constants"
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
	// ProxyPoolScopedResource represents if this request is asking for pool-scoped resources
	ProxyPoolScopedResource
	// DefaultYurtCoordinatorEtcdSvcName represents default yurt coordinator etcd service
	DefaultYurtCoordinatorEtcdSvcName = "yurt-coordinator-etcd"
	// DefaultYurtCoordinatorAPIServerSvcName represents default yurt coordinator apiServer service
	DefaultYurtCoordinatorAPIServerSvcName = "yurt-coordinator-apiserver"
	// DefaultYurtCoordinatorEtcdSvcPort represents default yurt coordinator etcd port
	DefaultYurtCoordinatorEtcdSvcPort = "2379"
	// DefaultYurtCoordinatorAPIServerSvcPort represents default yurt coordinator apiServer port
	DefaultYurtCoordinatorAPIServerSvcPort = "443"

	YurtHubNamespace      = "kube-system"
	CacheUserAgentsKey    = "cache_agents"
	PoolScopeResourcesKey = "pool_scope_resources"

	YurtHubProxyPort       = 10261
	YurtHubPort            = 10267
	YurtHubProxySecurePort = 10268
)

var (
	DefaultCacheAgents   = []string{"kubelet", "kube-proxy", "flanneld", "coredns", "raven-agent-ds", projectinfo.GetAgentName(), projectinfo.GetHubName(), coordinatorconstants.DefaultPoolScopedUserAgent}
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

// WithIfPoolScopedResource returns a copy of parent in which IfPoolScopedResource is set,
// indicating whether this request is asking for pool-scoped resources.
func WithIfPoolScopedResource(parent context.Context, ifPoolScoped bool) context.Context {
	return WithValue(parent, ProxyPoolScopedResource, ifPoolScoped)
}

// IfPoolScopedResourceFrom returns the value of IfPoolScopedResource indicating whether this request
// is asking for pool-scoped resource.
func IfPoolScopedResourceFrom(ctx context.Context) (bool, bool) {
	info, ok := ctx.Value(ProxyPoolScopedResource).(bool)
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

// WriteObject write object to response writer
func WriteObject(statusCode int, obj runtime.Object, w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		gv := schema.GroupVersion{
			Group:   info.APIGroup,
			Version: info.APIVersion,
		}
		negotiatedSerializer := serializer.YurtHubSerializer.GetNegotiatedSerializer(gv.WithResource(info.Resource))
		responsewriters.WriteObjectNegotiated(negotiatedSerializer, negotiation.DefaultEndpointRestrictions, gv, w, req, statusCode, obj)
		return nil
	}

	return fmt.Errorf("request info is not found when write object, %s", ReqString(req))
}

func NewTripleReadCloser(req *http.Request, rc io.ReadCloser, isRespBody bool) (io.ReadCloser, io.ReadCloser, io.ReadCloser) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	tr := &tripleReadCloser{
		req: req,
		rc:  rc,
		pw1: pw1,
		pw2: pw2,
	}
	return tr, pr1, pr2
}

type tripleReadCloser struct {
	req *http.Request
	rc  io.ReadCloser
	pw1 *io.PipeWriter
	pw2 *io.PipeWriter
	// isRespBody shows rc(is.ReadCloser) is a response.Body
	// or not(maybe a request.Body). if it is true(it's a response.Body),
	// we should close the response body in Close func, else not,
	// it(request body) will be closed by http request caller
	isRespBody bool
}

// Read read data into p and write into pipe
func (dr *tripleReadCloser) Read(p []byte) (n int, err error) {
	defer func() {
		if dr.req != nil && dr.isRespBody {
			ctx := dr.req.Context()
			info, _ := apirequest.RequestInfoFrom(ctx)
			if info.IsResourceRequest {
				comp, _ := ClientComponentFrom(ctx)
				metrics.Metrics.AddProxyTrafficCollector(comp, info.Verb, info.Resource, info.Subresource, n)
			}
		}
	}()

	n, err = dr.rc.Read(p)
	if n > 0 {
		var n1, n2 int
		var err error
		if n1, err = dr.pw1.Write(p[:n]); err != nil {
			klog.Errorf("tripleReader: could not write to pw1 %v", err)
			return n1, err
		}
		if n2, err = dr.pw2.Write(p[:n]); err != nil {
			klog.Errorf("tripleReader: could not write to pw2 %v", err)
			return n2, err
		}
	}

	return
}

// Close close two readers
func (dr *tripleReadCloser) Close() error {
	errs := make([]error, 0)
	if dr.isRespBody {
		if err := dr.rc.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if err := dr.pw1.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := dr.pw2.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return fmt.Errorf("could not close dualReader, %v", errs)
	}

	return nil
}

// NewDualReadCloser create an dualReadCloser object
func NewDualReadCloser(req *http.Request, rc io.ReadCloser, isRespBody bool) (io.ReadCloser, io.ReadCloser) {
	pr, pw := io.Pipe()
	dr := &dualReadCloser{
		req:        req,
		rc:         rc,
		pw:         pw,
		isRespBody: isRespBody,
	}

	return dr, pr
}

type dualReadCloser struct {
	req *http.Request
	rc  io.ReadCloser
	pw  *io.PipeWriter
	// isRespBody shows rc(is.ReadCloser) is a response.Body
	// or not(maybe a request.Body). if it is true(it's a response.Body),
	// we should close the response body in Close func, else not,
	// it(request body) will be closed by http request caller
	isRespBody bool
}

// Read read data into p and write into pipe
func (dr *dualReadCloser) Read(p []byte) (n int, err error) {
	defer func() {
		if dr.req != nil && dr.isRespBody {
			ctx := dr.req.Context()
			info, _ := apirequest.RequestInfoFrom(ctx)
			if info.IsResourceRequest {
				comp, _ := ClientComponentFrom(ctx)
				metrics.Metrics.AddProxyTrafficCollector(comp, info.Verb, info.Resource, info.Subresource, n)
			}
		}
	}()

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
	case WorkingModeCloud, WorkingModeEdge:
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
