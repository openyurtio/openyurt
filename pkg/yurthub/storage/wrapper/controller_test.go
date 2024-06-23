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
	"math/rand"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"

	fuzz "github.com/google/gofuzz"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

func Example() {

}

func TestHammerController(t *testing.T) {
	queue := NewQueueWithOptions()
	defer queue.Shutdown()
	store, err := disk.NewDiskStorage("/tmp/test/hammercontroller")
	if err != nil {
		t.Errorf("failed to initialize a store")
	}
	controller := NewController(queue, store)
	controller.Run(context.TODO(), 3)
	wg := sync.WaitGroup{}
	const threads = 3
	wg.Add(threads)
	for i := 0; i < threads; i++ {
		go func() {
			defer wg.Done()
			currentNames := sets.String{}
			rs := rand.NewSource(rand.Int63())
			f := fuzz.New().NilChance(.5).NumElements(0, 2).RandSource(rs)
			for i := 0; i < 10; i++ {
				pod := &v1.Pod{}
				var nameInt int
				var name string
				var isNew bool
				if currentNames.Len() == 0 || rand.Intn(3) == 1 {
					f.Fuzz(&nameInt)
					name = fmt.Sprintf("%d", nameInt)
					isNew = true
				} else {
					l := currentNames.List()
					name = l[rand.Intn(len(l))]
				}
				f.Fuzz(pod)
				pod.Name = name
				pod.Namespace = "default"
				pod.ResourceVersion = "0"
				key, _ := store.KeyFunc(storage.KeyBuildInfo{
					Component: "kubelet",
					Name:      name,
					Resources: "pods",
					Version:   "v1",
				})
				if isNew {
					currentNames.Insert(name)
					queue.Add(Item{
						Key:    key,
						Verb:   "create",
						Object: pod,
					})
					continue
				}
				switch rand.Intn(2) {
				case 0:
					// update
					currentNames.Insert(name)
					queue.Add(Item{
						Key:             key,
						Verb:            "update",
						Object:          pod,
						ResourceVersion: 1,
					})
				case 1:
					// delete
					currentNames.Delete(name)
					queue.Add(Item{
						Key:  key,
						Verb: "delete",
					})
				}
			}

		}()
	}
	wg.Wait()

	wait.PollUntilContextCancel(context.TODO(), time.Second, false,
		func(context.Context) (done bool, err error) {
			if queue.HasSynced() {
				return true, nil
			}
			return false, nil
		})
	if queue.Len() != 0 || len(queue.items) != 0 {
		t.Errorf("expect length of queue equals to 0")
	}
}
