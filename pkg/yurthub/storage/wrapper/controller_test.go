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

package wrapper

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func Example() {

}

func TestHammerController(t *testing.T) {
	queue := NewQueueWithOptions()
	store, err := disk.NewDiskStorage("/tmp/test/hammercontroller")
	if err != nil {
		t.Errorf("failed to initialize a store")
	}

	controller := NewController(queue, store)
	controller.Run(context.TODO(), 5)
	for i := 0; i < 10; i++ {
		key, _ := store.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Name:      fmt.Sprintf("%d", i),
			Resources: "pods",
			Version:   "v1",
		})
		queue.Add(Item{
			Key:    key,
			Verb:   "create",
			Object: &v1.Pod{},
		})
	}
	wait.PollUntilContextCancel(context.TODO(), time.Second, false,
		func(context.Context) (done bool, err error) {
			if queue.HasSynced() {
				return true, nil
			}
			return false, nil
		})
}
