/*
Copyright 2026 The OpenYurt Authors.

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

package gc

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	fakehealth "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

const testEventName = "test-event.abc123"

type testKey string

func (k testKey) Key() string {
	return string(k)
}

// storeWithFixedKeys overrides ListResourceKeysOfComponent to inject nil keys
// for covering GC nil-key guards.
type storeWithFixedKeys struct {
	storage.Store
	keys []storage.Key
}

func (s *storeWithFixedKeys) ListResourceKeysOfComponent(_ string, _ schema.GroupVersionResource) ([]storage.Key, error) {
	return s.keys, nil
}

func newTestGCManager(t *testing.T) (*GCManager, cachemanager.StorageWrapper, func()) {
	t.Helper()

	dir := fmt.Sprintf("/tmp/yurthub-gc-test-%d", time.Now().UnixNano())
	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}
	sWrapper := cachemanager.NewStorageWrapper(dStorage)
	mgr := &GCManager{
		store:    sWrapper,
		nodeName: "test-node",
	}
	return mgr, sWrapper, func() { _ = os.RemoveAll(dir) }
}

func cacheEventsV1Event(t *testing.T, sWrapper cachemanager.StorageWrapper, component, eventName string, obj *eventsv1.Event) storage.Key {
	t.Helper()

	eventKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Resources: "events",
		Namespace: "default",
		Name:      eventName,
		Group:     "events.k8s.io",
		Version:   "v1",
	})
	if err != nil {
		t.Fatalf("failed to build event key: %v", err)
	}
	if err := sWrapper.Create(eventKey, obj); err != nil {
		t.Fatalf("failed to create cached event: %v", err)
	}
	return eventKey
}

func newEventsV1Event(eventName string) *eventsv1.Event {
	return &eventsv1.Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "events.k8s.io/v1",
			Kind:       "Event",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventName,
			Namespace: "default",
		},
		EventTime:           metav1.NowMicro(),
		ReportingController: "kubelet",
		ReportingInstance:   "test-node",
		Action:              "Started",
		Reason:              "Started",
		Regarding: v1.ObjectReference{
			Kind: "Pod",
			Name: "mypod",
		},
	}
}

func newCoreV1Event(eventName string) *v1.Event {
	return &v1.Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Event",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventName,
			Namespace: "default",
		},
		Reason:  "Started",
		Message: "started",
		InvolvedObject: v1.ObjectReference{
			Kind: "Pod",
			Name: "mypod",
		},
	}
}

func listCachedEventKeys(t *testing.T, sWrapper cachemanager.StorageWrapper, component string) []storage.Key {
	t.Helper()

	keys, err := sWrapper.ListResourceKeysOfComponent(component, schema.GroupVersionResource{
		Group:    "events.k8s.io",
		Version:  "v1",
		Resource: "events",
	})
	if err != nil {
		t.Fatalf("failed to list cached event keys: %v", err)
	}
	return keys
}

func TestEventExistsOnAPIServer(t *testing.T) {
	eventName := testEventName
	eventsV1Obj := newEventsV1Event(eventName)
	coreV1Obj := newCoreV1Event(eventName)

	tests := []struct {
		name    string
		objects []runtime.Object
		want    bool
		wantErr bool
	}{
		{
			name:    "event exists in EventsV1",
			objects: []runtime.Object{eventsV1Obj},
			want:    true,
		},
		{
			name:    "event exists only in CoreV1",
			objects: []runtime.Object{coreV1Obj},
			want:    true,
		},
		{
			name:    "event missing from both APIs",
			objects: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(tt.objects...)
			got, err := eventExistsOnAPIServer(kubeClient, "default", eventName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("eventExistsOnAPIServer() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("eventExistsOnAPIServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGcEventsPreservesEventsV1OnlyEvent(t *testing.T) {
	mgr, sWrapper, cleanup := newTestGCManager(t)
	defer cleanup()

	eventObj := newEventsV1Event(testEventName)
	cacheEventsV1Event(t, sWrapper, "kubelet", testEventName, eventObj)

	kubeClient := fake.NewSimpleClientset(eventObj)
	if _, err := kubeClient.EventsV1().Events("default").Get(context.Background(), testEventName, metav1.GetOptions{}); err != nil {
		t.Fatalf("EventsV1 Get should succeed: %v", err)
	}
	if _, err := kubeClient.CoreV1().Events("default").Get(context.Background(), testEventName, metav1.GetOptions{}); err == nil {
		t.Fatal("CoreV1 Get should return NotFound for events.k8s.io-only event")
	}

	mgr.gcEvents(kubeClient, "kubelet")

	keys := listCachedEventKeys(t, sWrapper, "kubelet")
	if len(keys) != 1 {
		t.Fatalf("expected cached event to be preserved, got %d keys", len(keys))
	}
}

func TestGcEventsPreservesCoreV1OnlyEvent(t *testing.T) {
	mgr, sWrapper, cleanup := newTestGCManager(t)
	defer cleanup()

	eventObj := newEventsV1Event(testEventName)
	cacheEventsV1Event(t, sWrapper, "kubelet", testEventName, eventObj)

	coreEvent := newCoreV1Event(testEventName)
	kubeClient := fake.NewSimpleClientset(coreEvent)

	mgr.gcEvents(kubeClient, "kubelet")

	keys := listCachedEventKeys(t, sWrapper, "kubelet")
	if len(keys) != 1 {
		t.Fatalf("expected cached event to be preserved via CoreV1 fallback, got %d keys", len(keys))
	}
}

func TestGcEventsDeletesMissingEvent(t *testing.T) {
	mgr, sWrapper, cleanup := newTestGCManager(t)
	defer cleanup()

	eventObj := newEventsV1Event(testEventName)
	cacheEventsV1Event(t, sWrapper, "kubelet", testEventName, eventObj)

	kubeClient := fake.NewSimpleClientset()
	mgr.gcEvents(kubeClient, "kubelet")

	keys := listCachedEventKeys(t, sWrapper, "kubelet")
	if len(keys) != 0 {
		t.Fatalf("expected missing event to be gc'd, got %d keys", len(keys))
	}
}

func TestGcEventsSkipsInvalidKey(t *testing.T) {
	mgr, sWrapper, cleanup := newTestGCManager(t)
	defer cleanup()

	validEvent := newEventsV1Event(testEventName)
	validKey := cacheEventsV1Event(t, sWrapper, "kubelet", testEventName, validEvent)

	invalidKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Resources: "events",
		Namespace: "",
		Name:      "invalid-event",
		Group:     "events.k8s.io",
		Version:   "v1",
	})
	if err != nil {
		t.Fatalf("failed to build invalid event key: %v", err)
	}
	if err := sWrapper.Create(invalidKey, newEventsV1Event("invalid-event")); err != nil {
		t.Fatalf("failed to cache invalid event: %v", err)
	}

	kubeClient := fake.NewSimpleClientset(validEvent)
	mgr.gcEvents(kubeClient, "kubelet")

	keys := listCachedEventKeys(t, sWrapper, "kubelet")
	if len(keys) != 2 {
		t.Fatalf("expected both keys to remain listed, got %d", len(keys))
	}

	_, err = sWrapper.Get(validKey)
	if err != nil {
		t.Fatalf("valid cached event should be preserved: %v", err)
	}
	_, err = sWrapper.Get(invalidKey)
	if err != nil {
		t.Fatalf("invalid-key event should remain because gc skips it: %v", err)
	}
}

func TestGcEventsForKubeProxyComponent(t *testing.T) {
	mgr, sWrapper, cleanup := newTestGCManager(t)
	defer cleanup()

	eventObj := newEventsV1Event(testEventName)
	cacheEventsV1Event(t, sWrapper, "kube-proxy", testEventName, eventObj)

	kubeClient := fake.NewSimpleClientset(eventObj)
	mgr.gcEvents(kubeClient, "kube-proxy")

	keys := listCachedEventKeys(t, sWrapper, "kube-proxy")
	if len(keys) != 1 {
		t.Fatalf("expected kube-proxy cached event to be preserved, got %d keys", len(keys))
	}
}

func TestGcEventsNoOpWhenNoCachedEvents(t *testing.T) {
	mgr, _, cleanup := newTestGCManager(t)
	defer cleanup()

	kubeClient := fake.NewSimpleClientset()
	mgr.gcEvents(kubeClient, "kubelet")
}

func TestGCPodsWhenRestartWithCorruptCachePaths(t *testing.T) {
	dir, err := os.MkdirTemp("", "gc-pods-restart")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	diskStore, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}
	store := cachemanager.NewStorageWrapper(diskStore)

	podName := "pod-a"
	paths := []string{
		filepath.Join(dir, "kubelet", "pods.v1.core", "default", podName),
		filepath.Join(dir, "kubelet", "pods.v1.core", "default", "extra", "pod-b"),
		filepath.Join(dir, "kubelet", "pods.v1.core", "default", "tmp_pod-c"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	serverURL, err := url.Parse("https://127.0.0.1:6443")
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	nodeName := "node1"
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
	}
	kubeClient := fake.NewSimpleClientset(pod)
	healthChecker := fakehealth.NewFakeChecker(map[*url.URL]bool{serverURL: true})
	clientManager := transport.NewFakeTransportManager(200, map[string]kubernetes.Interface{
		serverURL.String(): kubeClient,
	})

	mgr := &GCManager{
		store:         store,
		nodeName:      nodeName,
		healthChecker: healthChecker,
		clientManager: clientManager,
	}

	mgr.gcPodsWhenRestart()

	key, err := diskStore.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Resources: "pods",
		Namespace: "default",
		Name:      podName,
		Group:     "",
		Version:   "v1",
	})
	if err != nil {
		t.Fatalf("failed to build pod key: %v", err)
	}

	if _, err := diskStore.Get(key); err != nil {
		t.Fatalf("expected cached pod to remain after gc, got error: %v", err)
	}
}

func TestGCPodsWhenRestartSkipsNilKeys(t *testing.T) {
	dir, err := os.MkdirTemp("", "gc-pods-nil-keys")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	diskStore, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}

	fixedStore := &storeWithFixedKeys{
		Store: diskStore,
		// Only a nil key: the nil guard must skip it without treating it as a deleted pod.
		keys: []storage.Key{nil},
	}
	store := cachemanager.NewStorageWrapper(fixedStore)

	serverURL, err := url.Parse("https://127.0.0.1:6443")
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	nodeName := "node1"
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-a",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
	}
	kubeClient := fake.NewSimpleClientset(pod)
	healthChecker := fakehealth.NewFakeChecker(map[*url.URL]bool{serverURL: true})
	clientManager := transport.NewFakeTransportManager(200, map[string]kubernetes.Interface{
		serverURL.String(): kubeClient,
	})

	mgr := &GCManager{
		store:         store,
		nodeName:      nodeName,
		healthChecker: healthChecker,
		clientManager: clientManager,
	}

	// Must not panic when a nil key is present in the local key list.
	mgr.gcPodsWhenRestart()
}

func TestGCEventsSkipsNilKeys(t *testing.T) {
	dir, err := os.MkdirTemp("", "gc-events-nil-keys")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	diskStore, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}

	fixedStore := &storeWithFixedKeys{
		Store: diskStore,
		keys:  []storage.Key{nil, testKey("kubelet/events.v1.events.k8s.io/default/stale-event")},
	}
	store := cachemanager.NewStorageWrapper(fixedStore)

	mgr := &GCManager{
		store:    store,
		nodeName: "node1",
	}

	// Must skip the nil key without panicking; unrecognized non-nil keys are logged and ignored.
	mgr.gcEvents(fake.NewSimpleClientset(), "kubelet")
}
