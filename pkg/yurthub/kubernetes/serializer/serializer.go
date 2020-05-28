package serializer

import (
	"fmt"
	"io"
	"mime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	restclientwatch "k8s.io/client-go/rest/watch"
	"k8s.io/klog"
)

// YurtHubSerializer is a global serializer manager for yurthub
var YurtHubSerializer = NewSerializerManager()

// SerializerManager is responsible for managing *rest.Serializers
type SerializerManager struct {
	// NegotiatedSerializer is used for obtaining encoders and decoders for multiple
	// supported media types.
	NegotiatedSerializer runtime.NegotiatedSerializer
}

// NewSerializerManager creates a *SerializerManager object with no version conversion
func NewSerializerManager() *SerializerManager {
	return &SerializerManager{
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}, // do not need version conversion
	}
}

// CreateSerializers create a *rest.Serializers for encoding or decoding runtime object
func (sm *SerializerManager) CreateSerializers(contentType, group, version string) (*rest.Serializers, error) {
	mediaTypes := sm.NegotiatedSerializer.SupportedMediaTypes()
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("the content type(%s) specified in the request is not recognized: %v", contentType, err)
	}

	info, ok := runtime.SerializerInfoForMediaType(mediaTypes, mediaType)
	if !ok {
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

	s := &rest.Serializers{
		Encoder: sm.NegotiatedSerializer.EncoderForVersion(info.Serializer, &reqGroupVersion),
		Decoder: sm.NegotiatedSerializer.DecoderToVersion(info.Serializer, internalGV),

		RenegotiatedDecoder: func(contentType string, params map[string]string) (runtime.Decoder, error) {
			info, ok := runtime.SerializerInfoForMediaType(mediaTypes, contentType)
			if !ok {
				return nil, fmt.Errorf("serializer for %s not registered", contentType)
			}
			return sm.NegotiatedSerializer.DecoderToVersion(info.Serializer, internalGV), nil
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
		return nil, nil
	}
	return out, nil
}

// WatchDecoder generates a Decoder for watch response
func WatchDecoder(serializers *rest.Serializers, body io.ReadCloser) (*restclientwatch.Decoder, error) {
	framer := serializers.Framer.NewFrameReader(body)
	streamingDecoder := streaming.NewDecoder(framer, serializers.StreamingSerializer)
	return restclientwatch.NewDecoder(streamingDecoder, serializers.Decoder), nil
}
