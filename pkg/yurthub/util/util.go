package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/alibaba/openyurt/pkg/yurthub/kubernetes/serializer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog"
)

type ProxyKeyType int

const (
	CanCacheHeader string = "Edge-Cache"

	ProxyReqContentType ProxyKeyType = iota
	ProxyRespContentType
	ProxyClientComponent
	ProxyReqCanCache
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

// ClientComponentFrom returns a copy of parent in which the client component value is set
func WithClientComponent(parent context.Context, component string) context.Context {
	return WithValue(parent, ProxyClientComponent, component)
}

// ClientComponentFrom returns the value of the client component key on the ctx
func ClientComponentFrom(ctx context.Context) (string, bool) {
	info, ok := ctx.Value(ProxyClientComponent).(string)
	return info, ok
}

// WithCanCacheAgent returns a copy of parent in which the request can cache value is set
func WithReqCanCache(parent context.Context, canCache bool) context.Context {
	return WithValue(parent, ProxyReqCanCache, canCache)
}

// CanCacheAgentFrom returns the value of the request can cache key on the ctx
func ReqCanCacheFrom(ctx context.Context) (bool, bool) {
	info, ok := ctx.Value(ProxyReqCanCache).(bool)
	return info, ok
}

func ReqString(req *http.Request) string {
	ctx := req.Context()
	comp, _ := ClientComponentFrom(ctx)
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		return fmt.Sprintf("%v %s %s: %s", comp, info.Verb, info.Resource, req.URL.String())
	}

	return fmt.Sprintf("%s of %s", comp, req.URL.String())
}

func ReqInfoString(info *apirequest.RequestInfo) string {
	if info == nil {
		return ""
	}

	return fmt.Sprintf("%s %s for %s", info.Verb, info.Resource, info.Path)
}

func WriteObject(statusCode int, obj runtime.Object, w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	gv := schema.GroupVersion{
		Group:   "",
		Version: runtime.APIVersionInternal,
	}
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		gv.Group = info.APIGroup
		gv.Version = info.APIVersion
	}

	responsewriters.WriteObjectNegotiated(serializer.YurtHubSerializer.NegotiatedSerializer, gv, w, req, statusCode, obj)
}

func Err(err error, w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	gv := schema.GroupVersion{
		Group:   "",
		Version: runtime.APIVersionInternal,
	}
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		gv.Group = info.APIGroup
		gv.Version = info.APIVersion
	}

	responsewriters.ErrorNegotiated(err, serializer.YurtHubSerializer.NegotiatedSerializer, gv, w, req)
}

func NewDualReadCloser(rc io.ReadCloser, isRespBody bool) (io.ReadCloser, io.ReadCloser) {
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

func (dr *dualReadCloser) Read(p []byte) (n int, err error) {
	n, err = dr.rc.Read(p)
	if n > 0 {
		if n, err := dr.pw.Write(p[:n]); err != nil {
			klog.Errorf("dualReader: failed to write %v", err)
			return n, err
		}
	}

	return
}

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
		return fmt.Errorf("failed to close dualReader, %v", errs)
	}

	return nil
}

func KeyFunc(comp, resource, ns, name string) (string, error) {
	if comp == "" || resource == "" {
		return "", fmt.Errorf("createKey: comp, resource can not be empty")
	}

	return filepath.Join(comp, resource, ns, name), nil
}

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

func IsSupportedLBMode(lbMode string) bool {
	switch lbMode {
	case "rr", "priority":
		return true
	}

	return false
}

func IsSupportedCertMode(certMode string) bool {
	switch certMode {
	case "kubelet":
		return true
	}

	return false
}
