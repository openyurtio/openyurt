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
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	minRequestTimeout = 300 * time.Second
)

var neverExitWatch <-chan time.Time = make(chan time.Time)

// realTimeoutFactory implements timeoutFactory
type realTimeoutFactory struct {
	timeout time.Duration
}

// TimeoutCh returns a channel which will receive something when the watch times out,
// and a cleanup function to call when this happens.
func (w *realTimeoutFactory) TimeoutCh() (<-chan time.Time, func() bool) {
	if w.timeout == 0 {
		return neverExitWatch, func() bool { return false }
	}
	t := time.NewTimer(w.timeout)
	return t.C, t.Stop
}

func (sp *shareProxy) shareWatch(w http.ResponseWriter, r *http.Request, gvr *schema.GroupVersionResource) {
	reqScope, err := sp.getReqScope(gvr)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	listOpts, err := sp.decodeListOptions(r, reqScope)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	storageOpts, err := sp.storageOpts(listOpts, gvr)
	if err != nil {
		util.Err(err, w, r)
	}

	timeout := getTimeout(&listOpts)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	outputMediaType, _, err := negotiation.NegotiateOutputMediaType(r, reqScope.Serializer, reqScope)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	rc, _, err := sp.cacheManager.ResourceCache(gvr)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	watcher, err := rc.Watch(ctx, "", *storageOpts)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	klog.V(3).InfoS("Starting watch", "path", r.URL.Path, "resourceVersion", listOpts.ResourceVersion, "labels", listOpts.LabelSelector, "fields", listOpts.FieldSelector, "timeout", timeout)
	serveWatch(newFilterWatch(watcher, sp.filterMgr.FindObjectFilters(r)), reqScope, outputMediaType, r, w, timeout)
}

func getTimeout(opts *metainternalversion.ListOptions) time.Duration {
	timeout := time.Duration(0)
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	if timeout == 0 && minRequestTimeout > 0 {
		timeout = time.Duration(float64(minRequestTimeout) * (rand.Float64() + 1.0))
	}
	return timeout
}

func serveWatch(watcher watch.Interface, scope *handlers.RequestScope, mediaTypeOptions negotiation.MediaTypeOptions, req *http.Request, w http.ResponseWriter, timeout time.Duration) {
	defer watcher.Stop()

	options, err := optionsForTransform(mediaTypeOptions, req)
	if err != nil {
		util.Err(err, w, req)
		return
	}

	// negotiate for the stream serializer from the scope's serializer
	serializer, err := negotiation.NegotiateOutputMediaTypeStream(req, scope.Serializer, scope)
	if err != nil {
		util.Err(err, w, req)
		return
	}

	framer := serializer.StreamSerializer.Framer
	streamSerializer := serializer.StreamSerializer.Serializer
	encoder := scope.Serializer.EncoderForVersion(streamSerializer, scope.Kind.GroupVersion())
	useTextFraming := serializer.EncodesAsText
	if framer == nil {
		util.Err(fmt.Errorf("no framer defined for %q available for embedded encoding", serializer.MediaType), w, req)
		return
	}
	// TODO: next step, get back mediaTypeOptions from negotiate and return the exact value here
	mediaType := serializer.MediaType
	if mediaType != runtime.ContentTypeJSON {
		mediaType += ";stream=watch"
	}

	// locate the appropriate embedded encoder based on the transform
	var embeddedEncoder runtime.Encoder
	contentKind, contentSerializer, transform := targetEncodingForTransform(scope, mediaTypeOptions, req)
	if transform {
		info, ok := runtime.SerializerInfoForMediaType(contentSerializer.SupportedMediaTypes(), serializer.MediaType)
		if !ok {
			util.Err(fmt.Errorf("no encoder for %q exists in the requested target %#v", serializer.MediaType, contentSerializer), w, req)
			return
		}
		embeddedEncoder = contentSerializer.EncoderForVersion(info.Serializer, contentKind.GroupVersion())
	} else {
		embeddedEncoder = scope.Serializer.EncoderForVersion(serializer.Serializer, contentKind.GroupVersion())
	}

	var serverShuttingDownCh <-chan struct{}
	if signals := request.ServerShutdownSignalFrom(req.Context()); signals != nil {
		serverShuttingDownCh = signals.ShuttingDown()
	}

	ctx := req.Context()

	server := &handlers.WatchServer{
		Watching: watcher,
		Scope:    scope,

		UseTextFraming:  useTextFraming,
		MediaType:       mediaType,
		Framer:          framer,
		Encoder:         encoder,
		EmbeddedEncoder: embeddedEncoder,

		Fixup: func(obj runtime.Object) runtime.Object {
			result, err := transformObject(ctx, obj, options, mediaTypeOptions, scope, req)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to transform object %v: %v", reflect.TypeOf(obj), err))
				return obj
			}
			// When we are transformed to a table, use the table options as the state for whether we
			// should print headers - on watch, we only want to print table headers on the first object
			// and omit them on subsequent events.
			if tableOptions, ok := options.(*metav1.TableOptions); ok {
				tableOptions.NoHeaders = true
			}
			return result
		},

		TimeoutFactory:       &realTimeoutFactory{timeout},
		ServerShuttingDownCh: serverShuttingDownCh,
	}

	server.ServeHTTP(w, req)
}

func optionsForTransform(mediaType negotiation.MediaTypeOptions, req *http.Request) (interface{}, error) {
	switch target := mediaType.Convert; {
	case target == nil:
	case target.Kind == "Table" && (target.GroupVersion() == metav1beta1.SchemeGroupVersion || target.GroupVersion() == metav1.SchemeGroupVersion):
		opts := &metav1.TableOptions{}
		if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, opts); err != nil {
			return nil, err
		}
		switch errs := validation.ValidateTableOptions(opts); len(errs) {
		case 0:
			return opts, nil
		case 1:
			return nil, errors.NewBadRequest(fmt.Sprintf("Unable to convert to Table as requested: %v", errs[0].Error()))
		default:
			return nil, errors.NewBadRequest(fmt.Sprintf("Unable to convert to Table as requested: %v", errs))
		}
	}
	return nil, nil
}

func targetEncodingForTransform(scope *handlers.RequestScope, mediaType negotiation.MediaTypeOptions, req *http.Request) (schema.GroupVersionKind, runtime.NegotiatedSerializer, bool) {
	switch target := mediaType.Convert; {
	case target == nil:
	case (target.Kind == "PartialObjectMetadata" || target.Kind == "PartialObjectMetadataList" || target.Kind == "Table") &&
		(target.GroupVersion() == metav1beta1.SchemeGroupVersion || target.GroupVersion() == metav1.SchemeGroupVersion):
		return *target, metainternalversionscheme.Codecs, true
	}
	return scope.Kind, scope.Serializer, false
}

func transformObject(ctx context.Context, obj runtime.Object, opts interface{}, mediaType negotiation.MediaTypeOptions, scope *handlers.RequestScope, req *http.Request) (runtime.Object, error) {
	if co, ok := obj.(runtime.CacheableObject); ok {
		if mediaType.Convert != nil {
			// Non-nil mediaType.Convert means that some conversion of the object
			// has to happen. Currently conversion may potentially modify the
			// object or assume something about it (e.g. asTable operates on
			// reflection, which won't work for any wrapper).
			// To ensure it will work correctly, let's operate on base objects
			// and not cache it for now.
			//
			// TODO: Long-term, transformObject should be changed so that it
			// implements runtime.Encoder interface.
			return doTransformObject(ctx, co.GetObject(), opts, mediaType, scope, req)
		}
	}
	return doTransformObject(ctx, obj, opts, mediaType, scope, req)
}

func doTransformObject(ctx context.Context, obj runtime.Object, opts interface{}, mediaType negotiation.MediaTypeOptions, scope *handlers.RequestScope, req *http.Request) (runtime.Object, error) {
	if _, ok := obj.(*metav1.Status); ok {
		return obj, nil
	}

	// ensure that for empty lists we don't return <nil> items.
	// This is safe to modify without deep-copying the object, as
	// List objects themselves are never cached.
	if meta.IsListType(obj) && meta.LenList(obj) == 0 {
		if err := meta.SetList(obj, []runtime.Object{}); err != nil {
			return nil, err
		}
	}

	switch target := mediaType.Convert; {
	case target == nil:
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
