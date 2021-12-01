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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
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
)

const (
	// YurtHubCertificateManagerName represents the certificateManager name in yurthub mode
	YurtHubCertificateManagerName = "hubself"
	// DefaultKubeletPairFilePath represents the default kubelet pair file path
	DefaultKubeletPairFilePath = "/var/lib/kubelet/pki/kubelet-client-current.pem"
	// DefaultKubeletRootCAFilePath represents the default kubelet ca file path
	DefaultKubeletRootCAFilePath = "/etc/kubernetes/pki/ca.crt"
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
	YurtHubNamespace   = "kube-system"
	CacheUserAgentsKey = "cache_agents"
)

var (
	DefaultCacheAgents   = []string{"kubelet", "kube-proxy", "flanneld", "coredns", projectinfo.GetAgentName(), projectinfo.GetHubName()}
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

// IsKubeletLeaseReq judge whether the request is a lease request from kubelet
func IsKubeletLeaseReq(req *http.Request) bool {
	ctx := req.Context()
	if comp, ok := ClientComponentFrom(ctx); !ok || comp != "kubelet" {
		return false
	}
	if info, ok := apirequest.RequestInfoFrom(ctx); !ok || info.Resource != "leases" {
		return false
	}
	return true
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
	case YurtHubCertificateManagerName:
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

// LoadKubeletRestClientConfig load *rest.Config for accessing healthyServer
func LoadKubeletRestClientConfig(healthyServer *url.URL, kubeletRootCAFilePath, kubeletPairFilePath string) (*rest.Config, error) {
	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPool(kubeletRootCAFilePath); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", kubeletRootCAFilePath, err)
	} else {
		tlsClientConfig.CAFile = kubeletRootCAFilePath
	}

	if can, _ := certutil.CanReadCertAndKey(kubeletPairFilePath, kubeletPairFilePath); !can {
		return nil, fmt.Errorf("error reading %s, certificate and key must be supplied as a pair", kubeletPairFilePath)
	}
	tlsClientConfig.KeyFile = kubeletPairFilePath
	tlsClientConfig.CertFile = kubeletPairFilePath

	return &rest.Config{
		Host:            healthyServer.String(),
		TLSClientConfig: tlsClientConfig,
	}, nil
}

func LoadRESTClientConfig(kubeconfig string) (*rest.Config, error) {
	// Load structured kubeconfig data from the given path.
	loader := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	loadedConfig, err := loader.Load()
	if err != nil {
		return nil, err
	}
	// Flatten the loaded data to a particular restclient.Config based on the current context.
	return clientcmd.NewNonInteractiveClientConfig(
		*loadedConfig,
		loadedConfig.CurrentContext,
		&clientcmd.ConfigOverrides{},
		loader,
	).ClientConfig()
}

func LoadKubeConfig(kubeconfig string) (*clientcmdapi.Config, error) {
	loader := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	loadedConfig, err := loader.Load()
	if err != nil {
		return nil, err
	}

	return loadedConfig, nil
}

func CreateKubeConfigFile(kubeClientConfig *rest.Config, kubeconfigPath string) error {
	// Get the CA data from the bootstrap client config.
	caFile, caData := kubeClientConfig.CAFile, []byte{}
	if len(caFile) == 0 {
		caData = kubeClientConfig.CAData
	}

	// Build resulting kubeconfig.
	kubeconfigData := clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   kubeClientConfig.Host,
			InsecureSkipTLSVerify:    kubeClientConfig.Insecure,
			CertificateAuthority:     caFile,
			CertificateAuthorityData: caData,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			ClientCertificate: kubeClientConfig.CertFile,
			ClientKey:         kubeClientConfig.KeyFile,
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	// Marshal to disk
	return clientcmd.WriteToFile(kubeconfigData, kubeconfigPath)
}
