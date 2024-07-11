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

package cachemanager

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
)

var (
	AOFPrefix = "/var/lib/" + projectinfo.GetHubName() + "/autonomy"
)

const (
	CompressThresh = 20
)

type errorKeys struct {
	sync.RWMutex
	keys  map[string]string
	queue workqueue.RateLimitingInterface
	file  *os.File
	count int
}

func NewErrorKeys() *errorKeys {
	ek := &errorKeys{
		keys:  make(map[string]string),
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "error-keys"),
	}
	err := os.MkdirAll(AOFPrefix, 0755)
	if err != nil {
		klog.Errorf("failed to create dir: %v", err)
		return ek
	}
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		klog.Errorf("failed to open file, persistency is disabled: %v", err)
		return ek
	}
	ek.file = file
	go ek.sync()
	go ek.compress()
	metrics.Metrics.SetErrorKeysPersistencyStatus(1)
	return ek
}

type operator string

const (
	PUT operator = "put"
	DEL operator = "del"
)

type operation struct {
	Operator operator
	Key      string
	Val      string
}

func (ek *errorKeys) put(key string, val string) {
	ek.Lock()
	defer ek.Unlock()
	ek.keys[key] = val
	metrics.Metrics.IncErrorKeysCount()
	ek.queue.AddRateLimited(operation{Operator: PUT, Key: key, Val: val})
}

func (ek *errorKeys) del(key string) {
	ek.Lock()
	defer ek.Unlock()
	if _, ok := ek.keys[key]; !ok {
		return
	}
	delete(ek.keys, key)
	metrics.Metrics.DecErrorKeysCount()
	ek.queue.AddRateLimited(operation{Operator: DEL, Key: key})
}

func (ek *errorKeys) aggregate() string {
	ek.RLock()
	defer ek.RUnlock()
	var messageList []string
	for _, val := range ek.keys {
		messageList = append(messageList, val)
	}
	msg := strings.Join(messageList, "\n")
	return msg
}

func (ek *errorKeys) length() int {
	ek.RLock()
	defer ek.RUnlock()
	return len(ek.keys)
}

func (ek *errorKeys) sync() {
	for ek.processNextOperator() {
	}
}

func (ek *errorKeys) processNextOperator() bool {
	item, quit := ek.queue.Get()
	if quit {
		return false
	}
	defer ek.queue.Done(item)
	op, ok := item.(operation)
	if !ok {
		ek.queue.Forget(item)
		return true
	}
	data, err := json.Marshal(op)
	if err != nil {
		klog.Errorf("failed to serialize and persist operation: %v", op)
		return false
	}
	ek.file.Write(append(data, '\n'))
	ek.file.Sync()
	ek.count++
	return true
}

func (ek *errorKeys) compress() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		if !ek.queue.ShuttingDown() {
			if ek.count > len(ek.keys)+CompressThresh {
				ek.rewrite()
			}
		} else {
			return
		}
	}
}

func (ek *errorKeys) rewrite() {
	ek.RLock()
	defer ek.RUnlock()
	count := 0
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "tmp_aof"), os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		klog.Errorf("failed to open file: %v", err)
		return
	}
	for key, val := range ek.keys {
		op := operation{
			Key:      key,
			Val:      val,
			Operator: PUT,
		}
		data, err := json.Marshal(op)
		if err != nil {
			return
		}
		file.Write(append(data, '\n'))
		count++
	}
	file.Sync()
	file.Close()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true,
		func(ctx context.Context) (bool, error) {
			if ek.queue.Len() == 0 {
				return true, nil
			}
			return false, nil
		})
	if err != nil {
		klog.Errorf("failed to wait for queue to be empty")
		return
	}
	ek.file.Close()

	err = os.Rename(filepath.Join(AOFPrefix, "tmp_aof"), filepath.Join(AOFPrefix, "aof"))
	if err != nil {
		klog.Errorf("failed to rename tmp_aof to aof, %v", err)
	}
	file, err = os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_RDWR, 0644)
	if err != nil {
		klog.ErrorS(err, "failed to open file", "name", filepath.Join(AOFPrefix, "aof"))
		metrics.Metrics.SetErrorKeysPersistencyStatus(0)
		ek.queue.ShutDown()
		return
	}
	ek.file = file
	ek.count = count
}

func (ek *errorKeys) recover() {
	var file *os.File
	var err error
	if ek.file == nil {
		file, err = os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_RDWR, 0644)
		if err != nil {
			return
		}
	} else {
		file = ek.file
	}
	scanner := bufio.NewScanner(file)
	var operations []operation
	for scanner.Scan() {
		bytes := scanner.Bytes()
		var operation operation
		json.Unmarshal(bytes, &operation)
		operations = append(operations, operation)
	}
	for _, op := range operations {
		switch op.Operator {
		case PUT:
			ek.keys[op.Key] = op.Val
		case DEL:
			delete(ek.keys, op.Key)
		}
	}
}
