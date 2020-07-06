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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/openyurt/pkg/yurthub/kubernetes/serializer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"
)

// ProxyKeyType represents the key in proxy request context
type ProxyKeyType int

const (
	// ProxyReqContentType represents request content type context key
	ProxyReqContentType ProxyKeyType = iota
	// ProxyRespContentType represents response content type context key
	ProxyRespContentType
	// ProxyClientComponent represents client component context key
	ProxyClientComponent
	// ProxyReqCanCache represents request can cache context key
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

	responsewriters.WriteObjectNegotiated(serializer.YurtHubSerializer.NegotiatedSerializer, negotiation.DefaultEndpointRestrictions, gv, w, req, statusCode, obj)
}

// Err write err to response writer
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

// NewDualReadCloser create an dualReadCloser object
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

// Read read data into p and write into pipe
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
		return fmt.Errorf("failed to close dualReader, %v", errs)
	}

	return nil
}

// KeyFunc combine comp resource ns name into a key
func KeyFunc(comp, resource, ns, name string) (string, error) {
	if comp == "" || resource == "" {
		return "", fmt.Errorf("createKey: comp, resource can not be empty")
	}

	return filepath.Join(comp, resource, ns, name), nil
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

// IsSupportedCertMode check cert mode is supported or not
func IsSupportedCertMode(certMode string) bool {
	switch certMode {
	case "kubelet":
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

// LoadKubeletRestClientConfig load *rest.Config for accessing healthyServer
func LoadKubeletRestClientConfig(healthyServer *url.URL) (*rest.Config, error) {
	const (
		pairFile   = "/var/lib/kubelet/pki/kubelet-client-current.pem"
		rootCAFile = "/etc/kubernetes/pki/ca.crt"
	)

	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPool(rootCAFile); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", rootCAFile, err)
	} else {
		tlsClientConfig.CAFile = rootCAFile
	}

	if can, _ := certutil.CanReadCertAndKey(pairFile, pairFile); !can {
		return nil, fmt.Errorf("error reading %s, certificate and key must be supplied as a pair", pairFile)
	}
	tlsClientConfig.KeyFile = pairFile
	tlsClientConfig.CertFile = pairFile

	return &rest.Config{
		Host:            healthyServer.String(),
		TLSClientConfig: tlsClientConfig,
	}, nil
}
