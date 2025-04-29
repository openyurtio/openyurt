/*
Copyright 2024 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package multiplexer

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"

	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	minRequestTimeout = 300 * time.Second
)

type multiplexerProxy struct {
	requestsMultiplexerManager *multiplexer.MultiplexerManager
	restMapperManager          *hubmeta.RESTMapperManager
	stop                       <-chan struct{}
}

func init() {
	// When parsing the FieldSelector in list/watch requests, the corresponding resource's conversion functions need to be used.
	// Here, the primary action is to introduce the conversion functions registered in the core/v1 resources into the scheme.
	v1.AddToScheme(scheme.Scheme)
}

func NewMultiplexerProxy(multiplexerManager *multiplexer.MultiplexerManager, restMapperMgr *hubmeta.RESTMapperManager, stop <-chan struct{}) http.Handler {
	return &multiplexerProxy{
		stop:                       stop,
		requestsMultiplexerManager: multiplexerManager,
		restMapperManager:          restMapperMgr,
	}
}

func (sp *multiplexerProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqInfo, _ := request.RequestInfoFrom(r.Context())
	gvr := &schema.GroupVersionResource{
		Group:    reqInfo.APIGroup,
		Version:  reqInfo.APIVersion,
		Resource: reqInfo.Resource,
	}

	if !sp.requestsMultiplexerManager.Ready(gvr) {
		w.Header().Set("Retry-After", "1")
		util.Err(apierrors.NewTooManyRequestsError(fmt.Sprintf("cacher for gvr(%s) is initializing, please try again later.", gvr.String())), w, r)
		return
	}

	restStore, err := sp.requestsMultiplexerManager.ResourceStore(gvr)
	if err != nil {
		util.Err(errors.Wrapf(err, "failed to get rest storage"), w, r)
	}

	reqScope, err := sp.getReqScope(gvr)
	if err != nil {
		util.Err(errors.Wrapf(err, "failed tp get req scope"), w, r)
	}

	lister := restStore.(rest.Lister)
	watcher := restStore.(rest.Watcher)
	forceWatch := reqInfo.Verb == "watch"
	handlers.ListResource(lister, watcher, reqScope, forceWatch, minRequestTimeout).ServeHTTP(w, r)
}

func (sp *multiplexerProxy) getReqScope(gvr *schema.GroupVersionResource) (*handlers.RequestScope, error) {
	_, fqKindToRegister := sp.restMapperManager.KindFor(*gvr)
	if fqKindToRegister.Empty() {
		return nil, fmt.Errorf("gvk is not found for gvr: %v", *gvr)
	}

	return &handlers.RequestScope{
		Serializer:      scheme.Codecs,
		ParameterCodec:  scheme.ParameterCodec,
		Convertor:       scheme.Scheme,
		Defaulter:       scheme.Scheme,
		Typer:           scheme.Scheme,
		UnsafeConvertor: runtime.UnsafeObjectConvertor(scheme.Scheme),
		Authorizer:      authorizerfactory.NewAlwaysAllowAuthorizer(),

		EquivalentResourceMapper: runtime.NewEquivalentResourceRegistry(),

		// TODO: Check for the interface on storage
		TableConvertor: rest.NewDefaultTableConvertor(gvr.GroupResource()),

		// TODO: This seems wrong for cross-group subresources. It makes an assumption that a subresource and its parent are in the same group version. Revisit this.
		Resource: *gvr,
		Kind:     fqKindToRegister,

		HubGroupVersion: schema.GroupVersion{Group: fqKindToRegister.Group, Version: runtime.APIVersionInternal},

		MetaGroupVersion: metav1.SchemeGroupVersion,

		MaxRequestBodyBytes: int64(3 * 1024 * 1024),
		Namer: handlers.ContextBasedNaming{
			Namer: runtime.Namer(meta.NewAccessor()),
		},
	}, nil
}
