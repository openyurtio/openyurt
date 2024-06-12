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
	"sync"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

type Interface interface {
	Add(item Item)
	Replace(items Items)
	Get() (storage.Key, Items, bool)
	Len() int
	Done(key storage.Key)
	Shutdown()
	ShuttingDown() bool
	HasSynced() bool
}

type Item struct {
	Key             storage.Key
	Verb            string
	Object          runtime.Object
	ResourceVersion uint64
}

type Items []Item

type set map[storage.Key]struct{}

func (s set) has(item storage.Key) bool {
	_, exists := s[item]
	return exists
}

func (s set) insert(item storage.Key) {
	s[item] = struct{}{}
}

func (s set) delete(item storage.Key) {
	delete(s, item)
}

func (s set) len() int {
	return len(s)
}

type Queue struct {
	cond         *sync.Cond
	items        map[storage.Key]Items
	queue        []storage.Key
	dirty        set
	shuttingDown bool
}

func NewQueueWithOptions() *Queue {
	return &Queue{
		cond:  sync.NewCond(&sync.Mutex{}),
		dirty: set{},
		items: make(map[storage.Key]Items),
		queue: make([]storage.Key, 0),
	}
}

func (q *Queue) Get() (storage.Key, Items, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	if len(q.queue) == 0 {
		return nil, nil, true
	}

	id := q.queue[0]
	q.queue = q.queue[1:]

	q.dirty.delete(id)
	return id, q.items[id], false
}

func (q *Queue) Add(item Item) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if q.shuttingDown {
		return
	}
	oldItems := q.items[item.Key]
	var newItems Items
	if item.Object != nil {
		newItems = append(oldItems, item)
	}
	if q.dirty.has(item.Key) {
		q.items[item.Key] = newItems
		// q.cond.Broadcast()
	} else {
		q.dirty.insert(item.Key)
		q.queue = append(q.queue, item.Key)
		q.items[item.Key] = newItems
	}
	q.cond.Signal()
}

func (q *Queue) Replace(items Items) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if q.shuttingDown {
		return
	}
	key := items[0].Key

	if q.dirty.has(key) {
		q.items[key] = items
	} else {
		q.dirty.insert(key)
		q.queue = append(q.queue, key)
		q.items[key] = items
	}
	q.cond.Signal()
}

func (q *Queue) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return len(q.queue)
}

func (q *Queue) Done(key storage.Key) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.dirty.has(key) {
		q.queue = append(q.queue, key)
		q.cond.Signal()
	} else {
		delete(q.items, key)
	}
}

func (q *Queue) Shutdown() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.shuttingDown = true
	q.cond.Broadcast()
}

func (q *Queue) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	return q.shuttingDown
}

func (q *Queue) HasSynced() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.dirty.len() == 0
}