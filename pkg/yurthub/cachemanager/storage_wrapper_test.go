/*
Copyright 2021 The OpenYurt Authors.

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

package cachemanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

func clearDir(dir string) error {
	return os.RemoveAll(dir)
}

var testPod = &v1.Pod{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Pod",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "mypod1",
		Namespace:       "default",
		ResourceVersion: "1",
	},
}

func TestStorageWrapper(t *testing.T) {
	dir := fmt.Sprintf("%s-%d", rootDir, time.Now().Unix())

	defer clearDir(dir)

	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)

	t.Run("Test create storage", func(t *testing.T) {
		key, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Name:      "mypod1",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to create key, %v", err)
		}
		err = sWrapper.Create(key, testPod)
		if err != nil {
			t.Errorf("failed to create obj, %v", err)
		}
		obj, err := sWrapper.Get(key)
		if err != nil {
			t.Errorf("failed to create obj, %v", err)
		}
		accessor := meta.NewAccessor()
		name, _ := accessor.Name(obj)
		if name != "mypod1" {
			t.Errorf("the name is not expected, expect mypod1, get %s", name)
		}
	})

	t.Run("Test update storage", func(t *testing.T) {
		key, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Name:      "mypod1",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to generate key, %v", err)
		}
		fresherPod := testPod.DeepCopy()
		fresherPod.ResourceVersion = "2"
		stalerPod := testPod.DeepCopy()
		stalerPod.ResourceVersion = "0"
		fresherRvUint64, err := strconv.ParseUint(fresherPod.ResourceVersion, 10, 64)
		if err != nil {
			t.Errorf("failed to parse fresher rv, %v", err)
		}
		stalerRvUint64, err := strconv.ParseUint(stalerPod.ResourceVersion, 10, 64)
		if err != nil {
			t.Errorf("failed to parse staler rv, %v", err)
		}
		obj, err := sWrapper.Update(key, fresherPod, fresherRvUint64)
		if err != nil {
			t.Errorf("failed to update obj, %v", err)
		}
		if !reflect.DeepEqual(obj, fresherPod) {
			t.Errorf("should got updated obj %v, but got obj %v", fresherPod, obj)
		}

		obj, err = sWrapper.Get(key)
		if err != nil {
			t.Errorf("unexpected error, %v", err)
		}
		if !reflect.DeepEqual(obj, fresherPod) {
			t.Errorf("got unexpected fresher obj, want %v, got %v", fresherPod, obj)
		}

		obj, err = sWrapper.Update(key, stalerPod, stalerRvUint64)
		if err != storage.ErrUpdateConflict {
			t.Errorf("want: %v, got: %v", storage.ErrUpdateConflict, err)
		}
		if !reflect.DeepEqual(obj, fresherPod) {
			t.Errorf("got unexpected existing obj, want: %v, got: %v", fresherPod, obj)
		}
	})

	t.Run("Test list key of empty objs", func(t *testing.T) {
		err := os.MkdirAll(filepath.Join(dir, "kubelet", "runtimeclasses.v1.node.k8s.io"), 0755)
		if err != nil {
			t.Errorf("failed to create dir, %v", err)
		}
		defer os.RemoveAll(filepath.Join(dir, "kubelet", "runtimeclasses.v1.node.k8s.io"))
		rootKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "runtimeclasses",
			Group:     "node.k8s.io",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to create key, %v", err)
		}
		objs, err := sWrapper.List(rootKey)
		if err != nil {
			t.Errorf("failed to list objs, %v", err)
		}
		if len(objs) != 0 {
			t.Errorf("unexpected objs num, expect: 0, got: %d", len(objs))
		}
	})

	t.Run("Test list keys and obj", func(t *testing.T) {
		// test an exist key
		keys, err := sWrapper.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		})
		if err != nil {
			t.Errorf("failed to list keys, %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("the length of keys is not expected, expect 1, get %d", len(keys))
		}

		// test a not exist key
		_, err = sWrapper.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
			Group:    "events.k8s.io",
			Version:  "v1",
			Resource: "events",
		})
		if err != storage.ErrStorageNotFound {
			t.Errorf("got unexpected error, want: %v, got: %v", storage.ErrStorageNotFound, err)
		}

		// test list obj
		rootKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to generate rootKey, %v", err)
		}
		_, err = sWrapper.List(rootKey)
		if err != nil {
			t.Errorf("failed to list obj, %v", err)
		}
	})

	t.Run("Test replace obj", func(t *testing.T) {
		podObj := testPod.DeepCopy()
		podKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Name:      podObj.Name,
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to generate key, %v", err)
		}

		err = sWrapper.ReplaceComponentList("kubelet", schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		}, "default", map[storage.Key]runtime.Object{
			podKey: podObj,
		})
		if err != nil {
			t.Errorf("failed to replace objs, %v", err)
		}
	})

	t.Run("Test delete storage", func(t *testing.T) {
		podKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Namespace: "default",
			Name:      "mypod1",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to generate key, %v", err)
		}
		err = sWrapper.Delete(podKey)
		if err != nil {
			t.Errorf("failed to delete obj, %v", err)
		}
		_, err = sWrapper.Get(podKey)
		if !errors.Is(err, storage.ErrStorageNotFound) {
			t.Errorf("unexpected error, %v", err)
		}
	})

	t.Run("Test list obj in empty path", func(t *testing.T) {
		rootKey, err := sWrapper.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "events",
			Namespace: "default",
			Group:     "",
			Version:   "v1",
		})
		if err != nil {
			t.Errorf("failed to generate key, %v", err)
		}
		_, err = sWrapper.List(rootKey)
		if !errors.Is(err, storage.ErrStorageNotFound) {
			t.Errorf("list obj got unexpected err, want: %v, got: %v", storage.ErrStorageNotFound, err)
		}
	})
}
