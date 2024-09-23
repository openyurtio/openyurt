/*
Copyright 2024 The OpenYurt Authors.

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

package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"
)

var (
	corev1GV    = schema.GroupVersion{Version: "v1"}
	corev1Codec = scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(corev1GV), scheme.Codecs.UniversalDecoder(corev1GV), corev1GV, corev1GV)

	discoveryGV      = schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"}
	discoveryv1Codec = scheme.Codecs.CodecForVersions(scheme.Codecs.LegacyCodec(discoveryGV), scheme.Codecs.UniversalDecoder(discoveryGV), discoveryGV, discoveryGV)
)

func TestRestStore_GetList(t *testing.T) {
	t.Run(" list services", func(t *testing.T) {
		rs := &store{
			restClient: newFakeClient(corev1GV, mockServiceListBody(), newListHeader()),
		}

		getListObj := &corev1.ServiceList{}
		err := rs.GetList(context.Background(), "", storage.ListOptions{}, getListObj)

		assert.Nil(t, err)
		assert.Equal(t, 1, len(getListObj.Items))
	})

	t.Run("list endpointslices", func(t *testing.T) {
		rs := &store{
			restClient: newFakeClient(corev1GV, mockEndpointSlicesListBody(), newListHeader()),
		}

		getListObj := &discovery.EndpointSliceList{}
		err := rs.GetList(context.Background(), "", storage.ListOptions{}, getListObj)

		assert.Nil(t, err)
		assert.Equal(t, 1, len(getListObj.Items))
	})
}

func newListHeader() http.Header {
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}

func mockServiceListBody() []byte {
	str := runtime.EncodeOrDie(corev1Codec, newServiceList())
	return []byte(str)
}

func mockEndpointSlicesListBody() []byte {
	str := runtime.EncodeOrDie(discoveryv1Codec, newEndpointSliceList())
	return []byte(str)
}

func newServiceList() *corev1.ServiceList {
	return &corev1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		Items: []corev1.Service{
			*newService(),
		},
	}
}

func newFakeClient(gv schema.GroupVersion, body []byte, header http.Header) *fake.RESTClient {
	return &fake.RESTClient{
		GroupVersion:         gv,
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		}),
	}
}

func newEndpointSliceList() *discovery.EndpointSliceList {
	return &discovery.EndpointSliceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		Items: []discovery.EndpointSlice{
			newEndpointSlice(),
		},
	}
}

func newEndpointSlice() discovery.EndpointSlice {
	return discovery.EndpointSlice{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EndpointSlice",
			APIVersion: "discovery.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns-12345",
			Namespace: "kube-system",
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{"192.168.0.1"},
				NodeName:  newStringPointer("node1"),
			},
			{
				Addresses: []string{"192.168.1.1"},
				NodeName:  newStringPointer("node2"),
			},
			{
				Addresses: []string{"192.168.2.3"},
				NodeName:  newStringPointer("node3"),
			},
		},
	}
}

func newStringPointer(str string) *string {
	return &str
}

func TestRestStore_Watch(t *testing.T) {
	rs := &store{
		restClient: newFakeClient(corev1GV, mockServiceWatchBody(), newWatchHeader()),
	}

	resultCh, err := rs.Watch(context.Background(), "", storage.ListOptions{})
	event := <-resultCh.ResultChan()

	assert.Nil(t, err)
	assert.Equal(t, event.Type, watch.Added)
}

func newWatchHeader() http.Header {
	header := http.Header{}
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return header
}

func mockServiceWatchBody() []byte {
	serializer := scheme.Codecs.SupportedMediaTypes()[0]
	framer := serializer.StreamSerializer.Framer
	streamSerializer := serializer.StreamSerializer.Serializer
	encoder := scheme.Codecs.EncoderForVersion(streamSerializer, corev1GV)

	buf := &bytes.Buffer{}
	fb := framer.NewFrameWriter(buf)

	e := streaming.NewEncoder(fb, encoder)

	e.Encode(newOutEvent(newService()))

	return buf.Bytes()
}

func newOutEvent(object runtime.Object) *metav1.WatchEvent {
	internalEvent := metav1.InternalEvent{
		Type:   watch.Added,
		Object: object,
	}

	outEvent := &metav1.WatchEvent{}
	metav1.Convert_v1_InternalEvent_To_v1_WatchEvent(&internalEvent, outEvent, nil)

	return outEvent
}

func TestRestStore_Versioner(t *testing.T) {
	rs := &store{}

	assert.Nil(t, rs.Versioner())
}

func TestRestStore_Create(t *testing.T) {
	rs := &store{}
	err := rs.Create(context.TODO(), "", newService(), newService(), 1)

	assert.Equal(t, ErrNoSupport, err)
}

func TestRestStore_Delete(t *testing.T) {
	rs := &store{}
	err := rs.Delete(context.TODO(), "", newService(), nil, nil, nil)

	assert.Equal(t, ErrNoSupport, err)
}

func TestRestStore_Get(t *testing.T) {
	rs := &store{}
	err := rs.Get(context.TODO(), "", storage.GetOptions{}, nil)

	assert.Equal(t, ErrNoSupport, err)
}

func TestRestStore_GuaranteedUpdate(t *testing.T) {
	rs := &store{}
	err := rs.GuaranteedUpdate(context.TODO(), "", newService(), false, nil, nil, nil)

	assert.Equal(t, ErrNoSupport, err)
}

func TestRestStore_Count(t *testing.T) {
	rs := &store{}
	_, err := rs.Count("")

	assert.Equal(t, ErrNoSupport, err)
}

func TestRestStore_RequestWatchProgress(t *testing.T) {
	rs := &store{}
	err := rs.RequestWatchProgress(context.TODO())

	assert.Equal(t, ErrNoSupport, err)
}

func newService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-dns",
			Namespace: "kube-system",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "192.168.0.10",
		},
	}
}
