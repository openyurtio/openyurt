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
	"net/http"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type multiplexerProxy struct {
	requestsMultiplexerManager multiplexer.MultiplexerManager
	filterFinder               filter.FilterFinder
	stop                       <-chan struct{}
}

func NewMultiplexerProxy(filterFinder filter.FilterFinder,
	cacheManager multiplexer.MultiplexerManager,
	multiplexerResources []schema.GroupVersionResource,
	stop <-chan struct{}) (*multiplexerProxy, error) {

	sp := &multiplexerProxy{
		stop:                       stop,
		requestsMultiplexerManager: cacheManager,
		filterFinder:               filterFinder,
	}

	for _, gvr := range multiplexerResources {
		if _, _, err := sp.requestsMultiplexerManager.ResourceCache(&gvr); err != nil {
			return sp, errors.Wrapf(err, "failed to init resource cache for %s", gvr.String())
		}
	}

	return sp, nil
}

func (sp *multiplexerProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqInfo, _ := request.RequestInfoFrom(r.Context())
	gvr := sp.getRequestGVR(reqInfo)

	switch reqInfo.Verb {
	case "list":
		sp.multiplexerList(w, r, gvr)
	case "watch":
		sp.multiplexerWatch(w, r, gvr)
	default:
		util.Err(errors.Errorf("Multiplexer proxy does not support the request method %s", reqInfo.Verb), w, r)
	}
}

func (sp *multiplexerProxy) getRequestGVR(reqInfo *request.RequestInfo) *schema.GroupVersionResource {
	return &schema.GroupVersionResource{
		Group:    reqInfo.APIGroup,
		Version:  reqInfo.APIVersion,
		Resource: reqInfo.Resource,
	}
}

func (sp *multiplexerProxy) getReqScope(gvr *schema.GroupVersionResource) (*handlers.RequestScope, error) {
	fqKindToRegister, err := sp.findKind(gvr)
	if err != nil {
		return nil, err
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

func (sp *multiplexerProxy) findKind(gvr *schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	object, err := sp.newListObject(gvr)
	if err != nil {
		return schema.GroupVersionKind{}, errors.Wrapf(err, "failed to new list object")
	}

	fqKinds, _, err := scheme.Scheme.ObjectKinds(object)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	for _, fqKind := range fqKinds {
		if fqKind.Group == gvr.Group {
			return fqKind, nil
		}
	}

	return schema.GroupVersionKind{}, nil
}

func (sp *multiplexerProxy) decodeListOptions(req *http.Request, scope *handlers.RequestScope) (opts metainternalversion.ListOptions, err error) {
	if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err != nil {
		return opts, err
	}

	if errs := validation.ValidateListOptions(&opts, false); len(errs) > 0 {
		err := kerrors.NewInvalid(schema.GroupKind{Group: metav1.GroupName, Kind: "ListOptions"}, "", errs)
		return opts, err
	}

	if opts.FieldSelector != nil {
		fn := func(label, value string) (newLabel, newValue string, err error) {
			return scope.Convertor.ConvertFieldLabel(scope.Kind, label, value)
		}
		if opts.FieldSelector, err = opts.FieldSelector.Transform(fn); err != nil {
			return opts, kerrors.NewBadRequest(err.Error())
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
				return opts, kerrors.NewBadRequest("fieldSelector metadata.name doesn't match requested name")
			}
		} else {
			opts.FieldSelector = nameSelector
		}
	}

	return opts, nil
}

func (sp *multiplexerProxy) storageOpts(listOpts metainternalversion.ListOptions, gvr *schema.GroupVersionResource) (*kstorage.ListOptions, error) {
	p := sp.selectionPredicate(listOpts, gvr)

	return &kstorage.ListOptions{
		ResourceVersion:      getResourceVersion(listOpts),
		ResourceVersionMatch: listOpts.ResourceVersionMatch,
		Recursive:            isRecursive(p),
		Predicate:            p,
		SendInitialEvents:    listOpts.SendInitialEvents,
	}, nil
}

func (sp *multiplexerProxy) selectionPredicate(listOpts metainternalversion.ListOptions, gvr *schema.GroupVersionResource) kstorage.SelectionPredicate {
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
		GetAttrs:            sp.getAttrFunc(gvr),
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

func (sp *multiplexerProxy) getAttrFunc(gvr *schema.GroupVersionResource) kstorage.AttrFunc {
	rcc, err := sp.requestsMultiplexerManager.ResourceCacheConfig(gvr)
	if err != nil {
		klog.Errorf("failed to get cache config for %v, error: %v", gvr, err)
		return nil
	}

	return rcc.GetAttrsFunc
}
