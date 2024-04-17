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

	"k8s.io/klog/v2"
)

const (
	AOFPrefix = "/tmp/errorkeys"
)

type errorKeys struct {
	sync.RWMutex
	keys       map[string]string
	operations chan operation
	file       *os.File
	count      int
	cancel     context.CancelFunc
}

func NewErrorKeys() *errorKeys {
	ek := &errorKeys{
		keys:       make(map[string]string),
		operations: make(chan operation, 10),
	}
	err := os.MkdirAll(AOFPrefix, 0755)
	if err != nil {
		klog.Errorf("failed to create dir: %v", err)
	}
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		klog.Error("failed to open file, persistency is disabled: %v", err)
		return ek
	}
	ek.file = file
	ctx, cancel := context.WithCancel(context.Background())
	ek.cancel = cancel
	go ek.sync(ctx)
	go ek.compress(ctx)
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
	ek.operations <- operation{Operator: PUT, Key: key, Val: val}
}

func (ek *errorKeys) del(key string) {
	ek.Lock()
	defer ek.Unlock()
	delete(ek.keys, key)
	ek.operations <- operation{Operator: DEL, Key: key}
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

func (ek *errorKeys) sync(ctx context.Context) {
	for {
		select {
		case op := <-ek.operations:
			data, err := json.Marshal(op)
			if err != nil {
				klog.Errorf("failed to serialize operation: %v", op)
			}
			ek.file.Write(append(data, '\n'))
			ek.file.Sync()
			ek.count++
		case <-ctx.Done():
			ek.file.Close()
			return
		}
	}
}

func (ek *errorKeys) compress(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ticker.C:
			if ek.count > 100 {
				ek.rewrite()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (ek *errorKeys) rewrite() {
	ek.RLock()
	defer ek.RUnlock()
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "tmp_aof"), os.O_CREATE|os.O_RDWR, 0600)
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
	}
	file.Sync()
	file.Close()
	ek.file.Close()

	err = os.Rename(filepath.Join(AOFPrefix, "aof"), filepath.Join(AOFPrefix, "tmp_aof"))
	if err != nil {
		ek.cancel()
		return
	}
	file, err = os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_RDWR, 0600)
	if err != nil {
		ek.cancel()
		return
	}
	ek.file = file
	ek.count = 0
}

func (ek *errorKeys) recover() {
	var file *os.File
	var err error
	if ek.file == nil {
		file, err = os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_RDWR, 0600)
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
