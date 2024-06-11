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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/klog/v2"
)

// watchEmbeddedEncoder performs encoding of the embedded object.
//
// NOTE: watchEmbeddedEncoder is NOT thread-safe.
type watchEmbeddedEncoder struct {
	encoder runtime.Encoder

	ctx context.Context

	// target, if non-nil, configures transformation type.
	// The other options are ignored if target is nil.
	target       *schema.GroupVersionKind
	tableOptions *metav1.TableOptions
	scope        *handlers.RequestScope

	// identifier of the encoder, computed lazily
	identifier runtime.Identifier
}

func newWatchEmbeddedEncoder(ctx context.Context, encoder runtime.Encoder, target *schema.GroupVersionKind, tableOptions *metav1.TableOptions, scope *handlers.RequestScope) *watchEmbeddedEncoder {
	return &watchEmbeddedEncoder{
		encoder:      encoder,
		ctx:          ctx,
		target:       target,
		tableOptions: tableOptions,
		scope:        scope,
	}
}

// Encode implements runtime.Encoder interface.
func (e *watchEmbeddedEncoder) Encode(obj runtime.Object, w io.Writer) error {
	if co, ok := obj.(runtime.CacheableObject); ok {
		return co.CacheEncode(e.Identifier(), e.doEncode, w)
	}
	return e.doEncode(obj, w)
}

func (e *watchEmbeddedEncoder) doEncode(obj runtime.Object, w io.Writer) error {
	result, err := doTransformObject(e.ctx, obj, e.tableOptions, e.target, e.scope)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to transform object %v: %v", reflect.TypeOf(obj), err))
		result = obj
	}

	// When we are transforming to a table, use the original table options when
	// we should print headers only on the first object - headers should be
	// omitted on subsequent events.
	if e.tableOptions != nil && !e.tableOptions.NoHeaders {
		e.tableOptions.NoHeaders = true
		// With options change, we should recompute the identifier.
		// Clearing this will trigger lazy recompute when needed.
		e.identifier = ""
	}

	return e.encoder.Encode(result, w)
}

// Identifier implements runtime.Encoder interface.
func (e *watchEmbeddedEncoder) Identifier() runtime.Identifier {
	if e.identifier == "" {
		e.identifier = e.embeddedIdentifier()
	}
	return e.identifier
}

type watchEmbeddedEncoderIdentifier struct {
	Name      string              `json:"name,omitempty"`
	Encoder   string              `json:"encoder,omitempty"`
	Target    string              `json:"target,omitempty"`
	Options   metav1.TableOptions `json:"options,omitempty"`
	NoHeaders bool                `json:"noHeaders,omitempty"`
}

func (e *watchEmbeddedEncoder) embeddedIdentifier() runtime.Identifier {
	if e.target == nil {
		// If no conversion is performed, we effective only use
		// the embedded identifier.
		return e.encoder.Identifier()
	}
	identifier := watchEmbeddedEncoderIdentifier{
		Name:    "watch-embedded",
		Encoder: string(e.encoder.Identifier()),
		Target:  e.target.String(),
	}
	if e.target.Kind == "Table" && e.tableOptions != nil {
		identifier.Options = *e.tableOptions
		identifier.NoHeaders = e.tableOptions.NoHeaders
	}

	result, err := json.Marshal(identifier)
	if err != nil {
		klog.Fatalf("Failed marshaling identifier for watchEmbeddedEncoder: %v", err)
	}
	return runtime.Identifier(result)
}

// doTransformResponseObject is used for handling all requests, including watch.
func doTransformObject(ctx context.Context, obj runtime.Object, opts interface{}, target *schema.GroupVersionKind, scope *handlers.RequestScope) (runtime.Object, error) {
	if _, ok := obj.(*metav1.Status); ok {
		return obj, nil
	}

	switch {
	case target == nil:
		// If we ever change that from a no-op, the identifier of
		// the watchEmbeddedEncoder has to be adjusted accordingly.
		return obj, nil

	case target.Kind == "PartialObjectMetadata":
		return asPartialObjectMetadata(obj, target.GroupVersion())

	case target.Kind == "PartialObjectMetadataList":
		return asPartialObjectMetadataList(obj, target.GroupVersion())

	case target.Kind == "Table":
		options, ok := opts.(*metav1.TableOptions)
		if !ok {
			return nil, fmt.Errorf("unexpected TableOptions, got %T", opts)
		}
		return asTable(ctx, obj, options, scope, target.GroupVersion())

	default:
		accepted, _ := negotiation.MediaTypesForSerializer(metainternalversionscheme.Codecs)
		err := negotiation.NewNotAcceptableError(accepted)
		return nil, err
	}
}

func asTable(ctx context.Context, result runtime.Object, opts *metav1.TableOptions, scope *handlers.RequestScope, groupVersion schema.GroupVersion) (runtime.Object, error) {
	switch groupVersion {
	case metav1beta1.SchemeGroupVersion, metav1.SchemeGroupVersion:
	default:
		return nil, newNotAcceptableError(fmt.Sprintf("no Table exists in group version %s", groupVersion))
	}

	obj, err := scope.TableConvertor.ConvertToTable(ctx, result, opts)
	if err != nil {
		return nil, err
	}

	table := (*metav1.Table)(obj)

	for i := range table.Rows {
		item := &table.Rows[i]
		switch opts.IncludeObject {
		case metav1.IncludeObject:
			item.Object.Object, err = scope.Convertor.ConvertToVersion(item.Object.Object, scope.Kind.GroupVersion())
			if err != nil {
				return nil, err
			}
		// TODO: rely on defaulting for the value here?
		case metav1.IncludeMetadata, "":
			m, err := meta.Accessor(item.Object.Object)
			if err != nil {
				return nil, err
			}
			// TODO: turn this into an internal type and do conversion in order to get object kind automatically set?
			partial := meta.AsPartialObjectMetadata(m)
			partial.GetObjectKind().SetGroupVersionKind(groupVersion.WithKind("PartialObjectMetadata"))
			item.Object.Object = partial
		case metav1.IncludeNone:
			item.Object.Object = nil
		default:
			err = errors.NewBadRequest(fmt.Sprintf("unrecognized includeObject value: %q", opts.IncludeObject))
			return nil, err
		}
	}

	return table, nil
}

// errNotAcceptable indicates Accept negotiation has failed
type errNotAcceptable struct {
	message string
}

func newNotAcceptableError(message string) error {
	return errNotAcceptable{message}
}

func (e errNotAcceptable) Error() string {
	return e.message
}

func (e errNotAcceptable) Status() metav1.Status {
	return metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusNotAcceptable,
		Reason:  metav1.StatusReason("NotAcceptable"),
		Message: e.Error(),
	}
}

func asPartialObjectMetadata(result runtime.Object, groupVersion schema.GroupVersion) (runtime.Object, error) {
	if meta.IsListType(result) {
		err := newNotAcceptableError(fmt.Sprintf("you requested PartialObjectMetadata, but the requested object is a list (%T)", result))
		return nil, err
	}
	switch groupVersion {
	case metav1beta1.SchemeGroupVersion, metav1.SchemeGroupVersion:
	default:
		return nil, newNotAcceptableError(fmt.Sprintf("no PartialObjectMetadataList exists in group version %s", groupVersion))
	}
	m, err := meta.Accessor(result)
	if err != nil {
		return nil, err
	}
	partial := meta.AsPartialObjectMetadata(m)
	partial.GetObjectKind().SetGroupVersionKind(groupVersion.WithKind("PartialObjectMetadata"))
	return partial, nil
}

func asPartialObjectMetadataList(result runtime.Object, groupVersion schema.GroupVersion) (runtime.Object, error) {
	li, ok := result.(metav1.ListInterface)
	if !ok {
		return nil, newNotAcceptableError(fmt.Sprintf("you requested PartialObjectMetadataList, but the requested object is not a list (%T)", result))
	}

	gvk := groupVersion.WithKind("PartialObjectMetadata")
	switch {
	case groupVersion == metav1beta1.SchemeGroupVersion:
		list := &metav1beta1.PartialObjectMetadataList{}
		err := meta.EachListItem(result, func(obj runtime.Object) error {
			m, err := meta.Accessor(obj)
			if err != nil {
				return err
			}
			partial := meta.AsPartialObjectMetadata(m)
			partial.GetObjectKind().SetGroupVersionKind(gvk)
			list.Items = append(list.Items, *partial)
			return nil
		})
		if err != nil {
			return nil, err
		}
		list.ResourceVersion = li.GetResourceVersion()
		list.Continue = li.GetContinue()
		list.RemainingItemCount = li.GetRemainingItemCount()
		return list, nil

	case groupVersion == metav1.SchemeGroupVersion:
		list := &metav1.PartialObjectMetadataList{}
		err := meta.EachListItem(result, func(obj runtime.Object) error {
			m, err := meta.Accessor(obj)
			if err != nil {
				return err
			}
			partial := meta.AsPartialObjectMetadata(m)
			partial.GetObjectKind().SetGroupVersionKind(gvk)
			list.Items = append(list.Items, *partial)
			return nil
		})
		if err != nil {
			return nil, err
		}
		list.ResourceVersion = li.GetResourceVersion()
		list.Continue = li.GetContinue()
		list.RemainingItemCount = li.GetRemainingItemCount()
		return list, nil

	default:
		return nil, newNotAcceptableError(fmt.Sprintf("no PartialObjectMetadataList exists in group version %s", groupVersion))
	}
}
