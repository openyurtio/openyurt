/*
Copyright 2025 The OpenYurt Authors.

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

package responsefilter

import (
	"bytes"
	"io"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

func encodeWatchEvents(t *testing.T, s *serializer.Serializer, n int) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	for i := 0; i < n; i++ {
		event := &watch.Event{
			Type: watch.Added,
			Object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
			},
		}
		if _, err := s.WatchEncode(buf, event); err != nil {
			t.Fatalf("failed to encode watch event: %v", err)
		}
	}
	return buf
}

func newWatchFilterReadCloser(rc io.ReadCloser, s *serializer.Serializer, stopCh <-chan struct{}) *filterReadCloser {
	return &filterReadCloser{
		rc:           rc,
		watchDataCh:  make(chan *bytes.Buffer),
		filterCache:  new(bytes.Buffer),
		serializer:   s,
		objectFilter: &nopObjectHandler{},
		isWatch:      true,
		ownerName:    "foo",
		stopCh:       stopCh,
		closeCh:      make(chan struct{}),
	}
}

func TestStreamResponseFilterExitsOnClose(t *testing.T) {
	sm := serializer.NewSerializerManager()
	s := sm.CreateSerializer("application/json", "", "v1", "services")
	if s == nil {
		t.Fatal("failed to create serializer")
	}

	frc := newWatchFilterReadCloser(io.NopCloser(encodeWatchEvents(t, s, 5)), s, make(chan struct{}))

	done := make(chan error, 1)
	go func() {
		done <- frc.streamResponseFilter(frc.rc, frc.watchDataCh)
	}()

	<-frc.watchDataCh

	if err := frc.Close(); err != nil {
		t.Fatalf("failed to close filter read closer: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error after Close, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamResponseFilter goroutine leaked: still blocked after Close")
	}
}

func TestStreamResponseFilterExitsOnStopCh(t *testing.T) {
	sm := serializer.NewSerializerManager()
	s := sm.CreateSerializer("application/json", "", "v1", "services")
	if s == nil {
		t.Fatal("failed to create serializer")
	}

	stopCh := make(chan struct{})
	frc := newWatchFilterReadCloser(io.NopCloser(encodeWatchEvents(t, s, 5)), s, stopCh)

	done := make(chan error, 1)
	go func() {
		done <- frc.streamResponseFilter(frc.rc, frc.watchDataCh)
	}()

	<-frc.watchDataCh
	close(stopCh)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error after stopCh closed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamResponseFilter goroutine leaked: still blocked after stopCh closed")
	}
}

func TestStreamResponseFilterDeliversAllEventsThenEOF(t *testing.T) {
	sm := serializer.NewSerializerManager()
	s := sm.CreateSerializer("application/json", "", "v1", "services")
	if s == nil {
		t.Fatal("failed to create serializer")
	}

	frc := newWatchFilterReadCloser(io.NopCloser(encodeWatchEvents(t, s, 3)), s, make(chan struct{}))

	done := make(chan error, 1)
	go func() {
		done <- frc.streamResponseFilter(frc.rc, frc.watchDataCh)
	}()

	count := 0
	for range frc.watchDataCh {
		count++
	}

	if count != 3 {
		t.Fatalf("expected 3 watch events delivered, got %d", count)
	}

	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("expected io.EOF after stream ends, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamResponseFilter did not return after normal stream end")
	}
}
