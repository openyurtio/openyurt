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
	"net/url"
	"os"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	fakehealth "github.com/openyurtio/openyurt/pkg/yurthub/healthchecker/fake"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

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
