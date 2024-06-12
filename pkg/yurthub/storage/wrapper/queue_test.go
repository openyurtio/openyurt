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
	"fmt"
	"sync"
	"testing"
	"time"
)

type testKey struct {
	key int
}

func (t *testKey) Key() string {
	return fmt.Sprintf("%v", t.key)
}
func (t *testKey) IsRootKey() bool {
	return false
}

func TestBasic(t *testing.T) {
	tests := []struct {
		queue *Queue
	}{
		{
			queue: NewQueueWithOptions(),
		},
	}

	for _, test := range tests {
		const producers = 50
		producerWG := sync.WaitGroup{}
		producerWG.Add(producers)
		for i := 0; i < producers; i++ {
			go func(i int) {
				defer producerWG.Done()
				for j := 0; j < 50; j++ {
					test.queue.Add(Item{
						Key: &testKey{
							key: i,
						},
					})
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		const consumers = 10
		consumerWG := sync.WaitGroup{}
		consumerWG.Add(consumers)
		for i := 0; i < consumers; i++ {
			go func(i int) {
				defer consumerWG.Done()
				for {
					key, _, quit := test.queue.Get()
					if quit {
						return
					}
					test.queue.Done(key)
				}
			}(i)
		}

		producerWG.Wait()
		test.queue.Shutdown()
		test.queue.Add(Item{})
		consumerWG.Wait()
		if test.queue.Len() != 0 {
			t.Errorf("Expected the queue to be empty, had: %v items", test.queue.Len())
		}
	}
}
