package responsefilter

import (
	"bytes"
	"io"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

type fakeObjectFilter struct{}

func (f *fakeObjectFilter) Name() string { return "fake-filter" }

func (f *fakeObjectFilter) SupportedResourceAndVerbs() map[string]sets.Set[string] {
	return map[string]sets.Set[string]{"pods": sets.New("watch")}
}

func (f *fakeObjectFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	return obj
}

type fakeWatchBody struct {
	*bytes.Reader
	closed chan struct{}
}

func (b *fakeWatchBody) Close() error {
	select {
	case <-b.closed:
		// already closed
	default:
		close(b.closed)
	}
	return nil
}

func newSerializer(t *testing.T) *serializer.Serializer {
	t.Helper()
	sm := serializer.NewSerializerManager()
	s := sm.CreateSerializer("application/json", "", "v1", "pods")
	if s == nil {
		t.Fatal("failed to create serializer for corev1 pods")
	}
	return s
}

func buildWatchStreamBytes(t *testing.T, s *serializer.Serializer, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	for i := 0; i < n; i++ {
		pod := &corev1.Pod{}
		pod.Name = "pod"
		event := &watch.Event{Type: watch.Added, Object: pod}
		if _, err := s.WatchEncode(&buf, event); err != nil {
			t.Fatalf("failed to encode watch event: %v", err)
		}
	}
	return buf.Bytes()
}

func TestStreamResponseFilter_ExitsWhenConsumerStopsReading(t *testing.T) {
	s := newSerializer(t)

	data := buildWatchStreamBytes(t, s, 5)
	body := &fakeWatchBody{
		Reader: bytes.NewReader(data),
		closed: make(chan struct{}),
	}

	stopCh := make(chan struct{})
	frc := &filterReadCloser{
		rc:           body,
		filterCache:  new(bytes.Buffer),
		watchDataCh:  make(chan *bytes.Buffer),
		serializer:   s,
		objectFilter: &fakeObjectFilter{},
		isWatch:      true,
		ownerName:    "test",
		stopCh:       stopCh,
	}

	done := make(chan error, 1)
	go func() {
		done <- frc.streamResponseFilter(frc.rc, frc.watchDataCh)
	}()

	<-frc.watchDataCh

	close(stopCh)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("FAILED: streamResponseFilter goroutine is stuck — fix not applied or not working")
	}
}

func TestStreamResponseFilter_NormalOperationStillWorks(t *testing.T) {
	s := newSerializer(t)
	data := buildWatchStreamBytes(t, s, 3)
	body := &fakeWatchBody{
		Reader: bytes.NewReader(data),
		closed: make(chan struct{}),
	}

	stopCh := make(chan struct{})
	frc := &filterReadCloser{
		rc:           body,
		filterCache:  new(bytes.Buffer),
		watchDataCh:  make(chan *bytes.Buffer),
		serializer:   s,
		objectFilter: &fakeObjectFilter{},
		isWatch:      true,
		ownerName:    "test",
		stopCh:       stopCh,
	}

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
			t.Fatalf("expected io.EOF after stream ends normally, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("streamResponseFilter did not return after normal stream end")
	}
}
