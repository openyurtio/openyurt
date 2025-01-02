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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func (sp *multiplexerProxy) multiplexerWatch(w http.ResponseWriter, r *http.Request, gvr *schema.GroupVersionResource) {
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

	rc, _, err := sp.requestsMultiplexerManager.ResourceCache(gvr)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	key, err := sp.getCacheKey(r, storageOpts)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	watcher, err := rc.Watch(ctx, key, *storageOpts)
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

	handler, err := serveWatchHandler(watcher, scope, mediaTypeOptions, req, timeout)
	if err != nil {
		util.Err(err, w, req)
		return
	}

	handler.ServeHTTP(w, req)
}

func serveWatchHandler(watcher watch.Interface, scope *handlers.RequestScope, mediaTypeOptions negotiation.MediaTypeOptions, req *http.Request, timeout time.Duration) (http.Handler, error) {

	options, err := optionsForTransform(mediaTypeOptions, req)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Errorf("failed to get options from transform, error: %v", err))
	}

	// negotiate for the stream serializer from the scope's serializer
	serializer, err := negotiation.NegotiateOutputMediaTypeStream(req, scope.Serializer, scope)
	if err != nil {
		return nil, errors.NewInternalError(fmt.Errorf("failed to get output media type stream, error: %v", err))
	}

	framer := serializer.StreamSerializer.Framer
	streamSerializer := serializer.StreamSerializer.Serializer
	encoder := scope.Serializer.EncoderForVersion(streamSerializer, scope.Kind.GroupVersion())
	useTextFraming := serializer.EncodesAsText
	if framer == nil {
		return nil, errors.NewInternalError(fmt.Errorf("no framer defined for %q available for embedded encoding", serializer.MediaType))
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
			return nil, errors.NewInternalError(fmt.Errorf("no encoder for %q exists in the requested target %#v", serializer.MediaType, contentSerializer))
		}
		embeddedEncoder = contentSerializer.EncoderForVersion(info.Serializer, contentKind.GroupVersion())
	} else {
		embeddedEncoder = scope.Serializer.EncoderForVersion(serializer.Serializer, contentKind.GroupVersion())
	}

	var memoryAllocator runtime.MemoryAllocator

	if encoderWithAllocator, supportsAllocator := embeddedEncoder.(runtime.EncoderWithAllocator); supportsAllocator {
		// don't put the allocator inside the embeddedEncodeFn as that would allocate memory on every call.
		// instead, we allocate the buffer for the entire watch session and release it when we close the connection.
		memoryAllocator = runtime.AllocatorPool.Get().(*runtime.Allocator)
		embeddedEncoder = runtime.NewEncoderWithAllocator(encoderWithAllocator, memoryAllocator)
	}

	var tableOptions *metav1.TableOptions
	if options != nil {
		if passedOptions, ok := options.(*metav1.TableOptions); ok {
			tableOptions = passedOptions
		} else {
			return nil, errors.NewInternalError(fmt.Errorf("unexpected options type: %T", options))
		}
	}
	embeddedEncoder = newWatchEmbeddedEncoder(req.Context(), embeddedEncoder, mediaTypeOptions.Convert, tableOptions, scope)

	var serverShuttingDownCh <-chan struct{}
	if signals := request.ServerShutdownSignalFrom(req.Context()); signals != nil {
		serverShuttingDownCh = signals.ShuttingDown()
	}

	server := &handlers.WatchServer{
		Watching: watcher,
		Scope:    scope,

		UseTextFraming:  useTextFraming,
		MediaType:       mediaType,
		Framer:          framer,
		Encoder:         encoder,
		EmbeddedEncoder: embeddedEncoder,

		TimeoutFactory:       &realTimeoutFactory{timeout},
		ServerShuttingDownCh: serverShuttingDownCh,
	}

	return http.HandlerFunc(server.HandleHTTP), nil
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
