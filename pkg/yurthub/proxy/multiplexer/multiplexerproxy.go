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

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type multiplexerProxy struct {
	requestsMultiplexerManager *multiplexer.MultiplexerManager
	filterFinder               filter.FilterFinder
	restMapperManager          *hubmeta.RESTMapperManager
	stop                       <-chan struct{}
}

func NewMultiplexerProxy(filterFinder filter.FilterFinder,
	multiplexerManager *multiplexer.MultiplexerManager,
	restMapperMgr *hubmeta.RESTMapperManager,
	stop <-chan struct{}) http.Handler {
	return &multiplexerProxy{
		stop:                       stop,
		requestsMultiplexerManager: multiplexerManager,
		filterFinder:               filterFinder,
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

	switch reqInfo.Verb {
	case "list":
		sp.multiplexerList(w, r, gvr)
	case "watch":
		sp.multiplexerWatch(w, r, gvr)
	default:
		util.Err(errors.Errorf("Multiplexer proxy does not support the request method %s", reqInfo.Verb), w, r)
	}
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

func (sp *multiplexerProxy) decodeListOptions(req *http.Request, scope *handlers.RequestScope) (opts metainternalversion.ListOptions, err error) {
	if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err != nil {
		return opts, err
	}

	if errs := validation.ValidateListOptions(&opts, false); len(errs) > 0 {
		err := apierrors.NewInvalid(schema.GroupKind{Group: metav1.GroupName, Kind: "ListOptions"}, "", errs)
		return opts, err
	}

	if opts.FieldSelector != nil {
		fn := func(label, value string) (newLabel, newValue string, err error) {
			return scope.Convertor.ConvertFieldLabel(scope.Kind, label, value)
		}
		if opts.FieldSelector, err = opts.FieldSelector.Transform(fn); err != nil {
			return opts, apierrors.NewBadRequest(err.Error())
		}
	}

	hasName := true
	_, name, err := scope.Namer.Name(req)
	if err != nil {
		hasName = false
	}

	if hasName {
		nameSelector := fields.OneTermEqualSelector("metadata.name", name)
		if opts.FieldSelector != nil && !opts.FieldSelector.Empty() {
			selectedName, ok := opts.FieldSelector.RequiresExactMatch("metadata.name")
			if !ok || name != selectedName {
				return opts, apierrors.NewBadRequest("fieldSelector metadata.name doesn't match requested name")
			}
		} else {
			opts.FieldSelector = nameSelector
		}
	}

	return opts, nil
}

func (sp *multiplexerProxy) storageOpts(listOpts metainternalversion.ListOptions) (*kstorage.ListOptions, error) {
	p := sp.selectionPredicate(listOpts)

	return &kstorage.ListOptions{
		ResourceVersion:      getResourceVersion(listOpts),
		ResourceVersionMatch: listOpts.ResourceVersionMatch,
		Recursive:            isRecursive(p),
		Predicate:            p,
		SendInitialEvents:    listOpts.SendInitialEvents,
	}, nil
}

func (sp *multiplexerProxy) selectionPredicate(listOpts metainternalversion.ListOptions) kstorage.SelectionPredicate {
	label := labels.Everything()
	if listOpts.LabelSelector != nil {
		label = listOpts.LabelSelector
	}

	field := fields.Everything()
	if listOpts.FieldSelector != nil {
		field = listOpts.FieldSelector
	}

	return kstorage.SelectionPredicate{
		Label:               label,
		Field:               field,
		Limit:               listOpts.Limit,
		Continue:            listOpts.Continue,
		GetAttrs:            multiplexer.AttrsFunc,
		AllowWatchBookmarks: listOpts.AllowWatchBookmarks,
	}
}

func getResourceVersion(opts metainternalversion.ListOptions) string {
	if opts.ResourceVersion == "" {
		return "0"
	}
	return opts.ResourceVersion
}

func isRecursive(p kstorage.SelectionPredicate) bool {
	if _, ok := p.MatchesSingle(); ok {
		return false
	}
	return true
}
