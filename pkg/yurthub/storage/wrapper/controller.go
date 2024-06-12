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
	"errors"
	iofs "io/fs"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

var (
	ConcurrentWorkers = 5
)

type Controller struct {
	queue Interface
	store storage.Store
}

func NewController(queue Interface, store storage.Store) *Controller {
	return &Controller{queue: queue, store: store}
}

func (c *Controller) Run(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}
}

func (c *Controller) worker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	key, items, quit := c.queue.Get()
	if quit {
		return false
	}
	err := c.syncHandler(ctx, key, items)
	c.handleErr(ctx, err, key)
	return true
}

func (c *Controller) syncHandler(ctx context.Context, key storage.Key, items Items) error {
	if key.IsRootKey() {
		objs := make(map[storage.Key]runtime.Object)
		for i := 0; i < len(items); i++ {
			objs[items[i].Key] = items[i].Object
		}
		return c.store.Replace(key, objs)
	}

	if len(items) == 0 {
		return nil
	}
	klog.Infof("key: %s", key.Key())
	item := items[len(items)-1]
	var err error
	switch item.Verb {
	case "create":
		err = c.store.Create(key, item.Object)
	case "update":
		_, err = c.store.Update(key, item.Object, item.ResourceVersion)
	case "delete":
		err = c.store.Delete(key)
	default:
		klog.Errorf("not supported verb: %s", item.Verb)
		return errors.New("only support create, update, delete, and list")
	}
	return err
}

func (c *Controller) handleErr(ctx context.Context, err error, key storage.Key) {
	switch {
	case errors.Is(err, storage.ErrStorageAccessConflict):
		c.queue.Add(Item{Key: key})

	case errors.Is(err, iofs.ErrPermission):
		klog.Errorf("failed to operate %s, permission denied: %v", key, err)
	case errors.Is(err, iofs.ErrClosed):
		klog.Errorf("failed to operate %s, file closed: %v", key, err)
	case errors.Is(err, iofs.ErrInvalid):
		klog.Errorf("failed to operate %s, invalid argument: %v", key, err)

	case errors.Is(err, fs.ErrExists):
		klog.Errorf("failed to operate %s, file exists: %v", key, err)
	case errors.Is(err, fs.ErrNotExists):
		klog.Errorf("failed to operate %s, file not exists: %v", key, err)
	case errors.Is(err, fs.ErrIsNotDir):
		klog.Errorf("failed to operate %s, path points to a file, not dir: %v", key, err)
	case errors.Is(err, fs.ErrIsNotFile):
		klog.Errorf("failed to operate %s, path points to a dir, not file: %v", key, err)
	case errors.Is(err, fs.ErrInvalidPath):
		klog.Errorf("failed to operate %s, path is invalid: %v", key, err)
	case errors.Is(err, fs.ErrSysCall):
		klog.ErrorS(err, "system call failed")
		c.queue.Add(Item{Key: key})

	case errors.Is(err, nil):
		c.queue.Done(key)
	default:
		klog.Errorf("failed to get/store %s: %v", key, err)
	}
}
