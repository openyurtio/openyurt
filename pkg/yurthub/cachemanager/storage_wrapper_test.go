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
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

func clearDir(dir string) error {
	return os.RemoveAll(dir)
}

var testPod = runtime.Object(&v1.Pod{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Pod",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "mypod1",
		Namespace:       "default",
		ResourceVersion: "1",
	},
})

func TestStorageWrapper(t *testing.T) {
	dir := fmt.Sprintf("%s-%d", rootDir, time.Now().Unix())

	defer clearDir(dir)

	dStorage, err := disk.NewDiskStorage(dir)
	if err != nil {
		t.Errorf("failed to create disk storage, %v", err)
	}
	sWrapper := NewStorageWrapper(dStorage)

	t.Run("Test create storage", func(t *testing.T) {
		err = sWrapper.Create("kubelet/pods/default/mypod1", testPod)
		if err != nil {
			t.Errorf("failed to create obj, %v", err)
		}
		obj, err := sWrapper.Get("kubelet/pods/default/mypod1")
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
		updatePod := runtime.Object(&v1.Pod{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "mypod1",
				Namespace:       "default",
				ResourceVersion: "1",
				Labels: map[string]string{
					"tag": "test",
				},
			},
		})
		err = sWrapper.Update("kubelet/pods/default/mypod1", updatePod)
		if err != nil {
			t.Errorf("failed to update obj, %v", err)
		}
		obj, err := sWrapper.Get("kubelet/pods/default/mypod1")
		if err != nil {
			t.Errorf("unexpected error, %v", err)
		}
		accessor := meta.NewAccessor()
		labels, _ := accessor.Labels(obj)
		if vaule, ok := labels["tag"]; ok {
			if vaule != "test" {
				t.Errorf("failed to get label, expect test, get %s", vaule)
			}
		} else {
			t.Errorf("unexpected error, the label `tag` is not existed")
		}

	})

	t.Run("Test list keys and obj", func(t *testing.T) {
		// test an exist key
		keys, err := sWrapper.ListKeys("kubelet/pods/default")
		if err != nil {
			t.Errorf("failed to list keys, %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("the length of keys is not expected, expect 1, get %d", len(keys))
		}

		// test a not exist key
		_, err = sWrapper.ListKeys("kubelet/pods/test")
		if err != nil {
			t.Errorf("failed to list keys, %v", err)
		}

		// test list obj
		_, err = sWrapper.List("kubelet/pods/default")
		if err != nil {
			t.Errorf("failed to list obj, %v", err)
		}
	})

	t.Run("Test replace obj", func(t *testing.T) {
		err = sWrapper.Replace("kubelet/pods/default", map[string]runtime.Object{
			"kubelet/pods/default/mypod1": runtime.Object(&v1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "mypod1",
					Namespace:       "default",
					ResourceVersion: "1",
					Labels: map[string]string{
						"tag": "test",
					},
				},
			}),
		})
		if err != nil {
			t.Errorf("failed to replace objs, %v", err)
		}
	})

	t.Run("Test delete storage", func(t *testing.T) {
		err = sWrapper.Delete("kubelet/pods/default/mypod1")
		if err != nil {
			t.Errorf("failed to delete obj, %v", err)
		}
		_, err = sWrapper.Get("kubelet/pods/default/mypod1")
		if !errors.Is(err, storage.ErrStorageNotFound) {
			t.Errorf("unexpected error, %v", err)
		}
	})

	t.Run("Test list obj in empty path", func(t *testing.T) {
		_, err = sWrapper.List("kubelet/pods/default")
		if !errors.Is(err, storage.ErrStorageNotFound) {
			t.Errorf("failed to list obj, %v", err)
		}
	})
}
