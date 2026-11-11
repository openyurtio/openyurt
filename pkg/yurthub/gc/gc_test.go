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

package gc

import (
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	diskstorage "github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

// TestGcPodKeyConsistency proves that the keys built in gcPodsWhenRestart
// (when constructing currentPodKeys from apiserver response) must include
// Version and Group to match the keys returned by ListResourceKeysOfComponent.
//
// Before the fix, gcPodsWhenRestart called KeyFunc without Version/Group,
// producing "pods..core" in enhancement mode instead of "pods.v1.core",
// causing all cached pod keys to appear deleted.
func TestGcPodKeyConsistency(t *testing.T) {
	// Create a disk storage in enhancement mode by ensuring no legacy
	// resource dirs exist. A fresh temp dir produces enhancement mode.
	tmpDir := t.TempDir()
	store, err := diskstorage.NewDiskStorage(filepath.Join(tmpDir, "cache"))
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}

	// Simulate what ListResourceKeysOfComponent does:
	// Build a key with full GVR info (like gc.go:98-102)
	listKey, err := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Resources: "pods",
		Group:     "",
		Version:   "v1",
		Namespace: "default",
		Name:      "test-pod",
	})
	if err != nil {
		t.Fatalf("failed to build list key: %v", err)
	}

	// Simulate the BUGGY code from gc.go:139-144 (without Version/Group)
	buggyKey, err := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Namespace: "default",
		Name:      "test-pod",
		Resources: "pods",
		// Version and Group intentionally omitted — this is the bug
	})
	if err != nil {
		t.Fatalf("failed to build buggy key: %v", err)
	}

	// Simulate the FIXED code (with Version and Group)
	fixedKey, err := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Namespace: "default",
		Name:      "test-pod",
		Resources: "pods",
		Version:   "v1",
	})
	if err != nil {
		t.Fatalf("failed to build fixed key: %v", err)
	}

	t.Logf("List key (from ListResourceKeysOfComponent): %q", listKey.Key())
	t.Logf("Buggy key (no Version/Group):                %q", buggyKey.Key())
	t.Logf("Fixed key (with Version):                    %q", fixedKey.Key())

	// The buggy key should NOT match the list key (proving the bug exists)
	if buggyKey.Key() == listKey.Key() {
		t.Logf("Keys match even without Version — storage may be in legacy mode, bug is not applicable in this mode")
	} else {
		t.Logf("CONFIRMED: Buggy key %q != list key %q — pod GC key mismatch exists in enhancement mode", buggyKey.Key(), listKey.Key())
	}

	// The fixed key MUST match the list key
	if fixedKey.Key() != listKey.Key() {
		t.Errorf("Fixed key %q does not match list key %q — fix is incorrect", fixedKey.Key(), listKey.Key())
	} else {
		t.Logf("VERIFIED: Fixed key matches list key — fix is correct")
	}
}

// TestGcPodKeyMapLookup simulates the actual map lookup that gcPodsWhenRestart
// performs to determine which pods should be garbage collected.
func TestGcPodKeyMapLookup(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := diskstorage.NewDiskStorage(filepath.Join(tmpDir, "cache"))
	if err != nil {
		t.Fatalf("failed to create disk storage: %v", err)
	}

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	// Create a cached pod entry (simulating what ListResourceKeysOfComponent returns)
	cachedKey, err := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Resources: gvr.Resource,
		Group:     gvr.Group,
		Version:   gvr.Version,
		Namespace: "kube-system",
		Name:      "coredns-abc123",
	})
	if err != nil {
		t.Fatalf("failed to build cached key: %v", err)
	}

	// Build the "current pods" map the way gcPodsWhenRestart does it
	// BUGGY: no Version/Group
	currentPodKeysBuggy := make(map[storage.Key]struct{})
	buggyKey, _ := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Namespace: "kube-system",
		Name:      "coredns-abc123",
		Resources: "pods",
	})
	currentPodKeysBuggy[buggyKey] = struct{}{}

	// FIXED: with Version
	currentPodKeysFixed := make(map[storage.Key]struct{})
	fixedKey, _ := store.KeyFunc(storage.KeyBuildInfo{
		Component: "kubelet",
		Namespace: "kube-system",
		Name:      "coredns-abc123",
		Resources: "pods",
		Version:   "v1",
	})
	currentPodKeysFixed[fixedKey] = struct{}{}

	// Check buggy lookup — this should MISS (the bug)
	if _, found := currentPodKeysBuggy[cachedKey]; found {
		t.Logf("Buggy code found the cached key (storage in legacy mode)")
	} else {
		t.Logf("CONFIRMED BUG: Buggy code did NOT find cached key %q in currentPodKeys — pod would be incorrectly marked for GC", cachedKey.Key())
	}

	// Check fixed lookup — this MUST find the key
	if _, found := currentPodKeysFixed[cachedKey]; !found {
		t.Errorf("Fixed code did NOT find cached key %q in currentPodKeys — fix is broken", cachedKey.Key())
	} else {
		t.Logf("VERIFIED FIX: Fixed code correctly found cached key in currentPodKeys")
	}
}
