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

package serializer

import (
	"fmt"
	"io"
	"mime"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured/unstructuredscheme"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	restclientwatch "k8s.io/client-go/rest/watch"
	"k8s.io/klog"
)

// YurtHubSerializer is a global serializer manager for yurthub
var YurtHubSerializer = NewSerializerManager()

// UnsafeDefaultRESTMapper is only used to check whether the GVK is in the scheme according to the GVR information
var UnsafeDefaultRESTMapper = NewDefaultRESTMapperFromScheme()

func NewDefaultRESTMapperFromScheme() *meta.DefaultRESTMapper {
	scheme := scheme.Scheme
	defaultGroupVersions := scheme.PrioritizedVersionsAllGroups()
	mapper := meta.NewDefaultRESTMapper(defaultGroupVersions)
	// enumerate all supported versions, get the kinds, and register with the mapper how to address
	// our resources.
	for _, gv := range defaultGroupVersions {
		for kind := range scheme.KnownTypes(gv) {
			//Since RESTMapper is only used for mapping GVR to GVK information,
			//the scope field is not involved in actual use, so all scope are currently set to meta.RESTScopeNamespace
			scope := meta.RESTScopeNamespace
			mapper.Add(gv.WithKind(kind), scope)
		}
	}
	return mapper
}

// SerializerManager is responsible for managing *rest.Serializers
type SerializerManager struct {
	// NegotiatedSerializer is used for obtaining encoders and decoders for multiple
	// supported media types.
	NegotiatedSerializer runtime.NegotiatedSerializer
	// UnstructuredNegotiatedSerializer is used to obtain encoders and decoders
	// for resources not registered in the scheme
	UnstructuredNegotiatedSerializer runtime.NegotiatedSerializer
}

// NewSerializerManager creates a *SerializerManager object with no version conversion
func NewSerializerManager() *SerializerManager {
	return &SerializerManager{
		// do not need version conversion, and keep the gvk information
		NegotiatedSerializer:             WithVersionCodecFactory{CodecFactory: scheme.Codecs},
		UnstructuredNegotiatedSerializer: NewUnstructuredNegotiatedSerializer(),
	}
}

// WithVersionCodecFactory is a CodecFactory that will explicitly ignore requests to perform conversion.
// It keeps the gvk during deserialization.
// This wrapper is used while code migrates away from using conversion (such as external clients)
type WithVersionCodecFactory struct {
	serializer.CodecFactory
}

// EncoderForVersion returns an encoder that does not do conversion, but does set the group version kind of the object
// when serialized.
func (f WithVersionCodecFactory) EncoderForVersion(serializer runtime.Encoder, version runtime.GroupVersioner) runtime.Encoder {
	return runtime.WithVersionEncoder{
		Version:     version,
		Encoder:     serializer,
		ObjectTyper: scheme.Scheme,
	}
}

// DecoderToVersion returns an decoder that does not do conversion, and keeps the gvk information
func (f WithVersionCodecFactory) DecoderToVersion(serializer runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return WithVersionDecoder{
		Decoder: serializer,
	}
}

// WithVersionDecoder keeps the group version kind of a deserialized object.
type WithVersionDecoder struct {
	runtime.Decoder
}

// Decode does not do conversion. It keeps the gvk during deserialization.
func (d WithVersionDecoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	return d.Decoder.Decode(data, defaults, into)
}

type UnstructuredNegotiatedSerializer struct {
	scheme  *runtime.Scheme
	typer   runtime.ObjectTyper
	creator runtime.ObjectCreater
}

// NewUnstructuredNegotiatedSerializer returns a negotiated serializer for Unstructured resources
func NewUnstructuredNegotiatedSerializer() runtime.NegotiatedSerializer {
	return UnstructuredNegotiatedSerializer{
		scheme:  scheme.Scheme,
		typer:   unstructuredscheme.NewUnstructuredObjectTyper(),
		creator: NewUnstructuredCreator(),
	}
}

func (s UnstructuredNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	return []runtime.SerializerInfo{
		{
			MediaType:        "application/json",
			MediaTypeType:    "application",
			MediaTypeSubType: "json",
			EncodesAsText:    true,
			Serializer:       json.NewSerializerWithOptions(json.DefaultMetaFactory, s.creator, s.typer, json.SerializerOptions{}),
			PrettySerializer: json.NewSerializerWithOptions(json.DefaultMetaFactory, s.creator, s.typer, json.SerializerOptions{Pretty: true}),
			StreamSerializer: &runtime.StreamSerializerInfo{
				EncodesAsText: true,
				Serializer:    json.NewSerializerWithOptions(json.DefaultMetaFactory, s.creator, s.typer, json.SerializerOptions{}),
				Framer:        json.Framer,
			},
		},
		{
			MediaType:        "application/yaml",
			MediaTypeType:    "application",
			MediaTypeSubType: "yaml",
			EncodesAsText:    true,
			Serializer:       json.NewSerializerWithOptions(json.DefaultMetaFactory, s.creator, s.typer, json.SerializerOptions{Yaml: true}),
		},
	}
}

//EncoderForVersion do nothing, but returns a encoder,
//if the object is unstructured, the encoder will encode object without conversion
func (s UnstructuredNegotiatedSerializer) EncoderForVersion(encoder runtime.Encoder, gv runtime.GroupVersioner) runtime.Encoder {
	return encoder
}

//DecoderToVersion do nothing, and returns a decoder that does not do conversion
func (s UnstructuredNegotiatedSerializer) DecoderToVersion(decoder runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return WithVersionDecoder{
		Decoder: decoder,
	}
}

type unstructuredCreator struct{}

// NewUnstructuredCreator returns a simple object creator that always returns an Unstructured or UnstructuredList
func NewUnstructuredCreator() runtime.ObjectCreater {
	return unstructuredCreator{}
}

func (c unstructuredCreator) New(kind schema.GroupVersionKind) (runtime.Object, error) {
	if strings.HasSuffix(kind.Kind, "List") {
		ret := &unstructured.UnstructuredList{}
		ret.SetGroupVersionKind(kind)
		return ret, nil
	} else {
		ret := &unstructured.Unstructured{}
		ret.SetGroupVersionKind(kind)
		return ret, nil
	}
}

// CreateSerializers create a *rest.Serializers for encoding or decoding runtime object
func (sm *SerializerManager) CreateSerializers(contentType, group, version, resource string) (*rest.Serializers, error) {
	var mediaTypes []runtime.SerializerInfo
	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}
	_, kindErr := UnsafeDefaultRESTMapper.KindFor(gvr)
	if kindErr == nil || resource == "WatchEvent" {
		mediaTypes = sm.NegotiatedSerializer.SupportedMediaTypes()
	} else {
		mediaTypes = sm.UnstructuredNegotiatedSerializer.SupportedMediaTypes()
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("the content type(%s) specified in the request is not recognized: %v", contentType, err)
	}
	info, ok := runtime.SerializerInfoForMediaType(mediaTypes, mediaType)
	if !ok {
		if mediaType == "application/vnd.kubernetes.protobuf" && kindErr != nil {
			return nil, fmt.Errorf("*unstructured.Unstructured(%s/%s) does not implement the protobuf marshalling interface and cannot be encoded to a protobuf message", group, version)
		}
		if len(contentType) != 0 || len(mediaTypes) == 0 {
			return nil, fmt.Errorf("no serializers registered for %s", contentType)
		}
		info = mediaTypes[0]
	}

	internalGV := schema.GroupVersions{
		{
			Group:   group,
			Version: runtime.APIVersionInternal,
		},
		// always include the legacy group as a decoding target to handle non-error `Status` return types
		{
			Group:   "",
			Version: runtime.APIVersionInternal,
		},
	}
	reqGroupVersion := schema.GroupVersion{
		Group:   group,
		Version: version,
	}
	var encoder runtime.Encoder
	var decoder runtime.Decoder
	if kindErr == nil {
		encoder = sm.NegotiatedSerializer.EncoderForVersion(info.Serializer, &reqGroupVersion)
		decoder = sm.NegotiatedSerializer.DecoderToVersion(info.Serializer, internalGV)
	} else {
		encoder = sm.UnstructuredNegotiatedSerializer.EncoderForVersion(info.Serializer, &reqGroupVersion)
		decoder = sm.UnstructuredNegotiatedSerializer.DecoderToVersion(info.Serializer, &reqGroupVersion)
	}

	s := &rest.Serializers{
		Encoder: encoder,
		Decoder: decoder,

		RenegotiatedDecoder: func(contentType string, params map[string]string) (runtime.Decoder, error) {
			info, ok := runtime.SerializerInfoForMediaType(mediaTypes, contentType)
			if !ok {
				return nil, fmt.Errorf("serializer for %s not registered", contentType)
			}
			if kindErr == nil {
				return sm.NegotiatedSerializer.DecoderToVersion(info.Serializer, internalGV), nil
			} else {
				return sm.UnstructuredNegotiatedSerializer.DecoderToVersion(info.Serializer, &reqGroupVersion), nil
			}
		},
	}
	if info.StreamSerializer != nil {
		s.StreamingSerializer = info.StreamSerializer.Serializer
		s.Framer = info.StreamSerializer.Framer
	}

	return s, nil
}

// DecodeResp decodes byte data into runtime object with specified serializers and content type
func DecodeResp(serializers *rest.Serializers, b []byte, reqContentType, respContentType string) (runtime.Object, error) {
	decoder := serializers.Decoder
	if len(respContentType) > 0 && (decoder == nil || (len(reqContentType) > 0 && respContentType != reqContentType)) {
		mediaType, params, err := mime.ParseMediaType(respContentType)
		if err != nil {
			return nil, fmt.Errorf("response content type(%s) is invalid, %v", respContentType, err)
		}
		decoder, err = serializers.RenegotiatedDecoder(mediaType, params)
		if err != nil {
			return nil, fmt.Errorf("response content type(%s) is not supported, %v", respContentType, err)
		}
		klog.Infof("serializer decoder changed from %s to %s(%v)", reqContentType, respContentType, params)
	}

	if len(b) == 0 {
		return nil, fmt.Errorf("0-length response with content type: %s", respContentType)
	}

	out, _, err := decoder.Decode(b, nil, nil)
	if err != nil {
		return nil, err
	}
	// if a different object is returned, see if it is Status and avoid double decoding
	// the object.
	switch out.(type) {
	case *metav1.Status:
		// it's not need to cache for status
		return out, nil
	}
	return out, nil
}

// CreateWatchDecoder generates a Decoder for watch response
func CreateWatchDecoder(contentType, group, version, resource string, body io.ReadCloser) (*restclientwatch.Decoder, error) {
	//get the general serializers to decode the watch event
	serializers, err := YurtHubSerializer.CreateSerializers(contentType, group, version, "WatchEvent")
	if err != nil {
		klog.Errorf("failed to create serializers in saveWatchObject, %v", err)
		return nil, err
	}

	//get the serializers to decode the embedded object inside watch event according to the GVR of embedded object
	embeddedSerializers, err := YurtHubSerializer.CreateSerializers(contentType, group, version, resource)
	if err != nil {
		klog.Errorf("failed to create serializers in saveWatchObject, %v", err)
		return nil, err
	}

	framer := serializers.Framer.NewFrameReader(body)
	streamingDecoder := streaming.NewDecoder(framer, serializers.StreamingSerializer)
	return restclientwatch.NewDecoder(streamingDecoder, embeddedSerializers.Decoder), nil
}
