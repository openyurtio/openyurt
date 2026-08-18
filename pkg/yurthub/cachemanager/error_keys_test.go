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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func TestXxx(t *testing.T) {
	testcases := []struct {
		name   string
		keys   []string
		err    []string
		length int
		info   []string
	}{
		{
			name: "test1",
			keys: []string{
				"kubelet",
				"flannel",
				"coredns",
			},
			err: []string{
				errors.New("fail1").Error(),
				errors.New("fail2").Error(),
				errors.New("fail3").Error(),
			},
			length: 3,
			info:   []string{"fail1", "fail2", "fail3"},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			aofPath, err := os.MkdirTemp("", "errorkeys")
			if err != nil {
				t.Errorf("failed to create dir: %v", err)
			}
			defer os.RemoveAll(aofPath)
			ek := NewErrorKeys(aofPath)
			for i := range tc.keys {
				ek.put(tc.keys[i], tc.err[i])
			}
			if ek.length() != tc.length {
				t.Errorf("expect length %v, got %v", tc.length, ek.length())
			}
			msg := ek.aggregate()
			for i := range tc.info {
				if !strings.Contains(msg, tc.info[i]) {
					t.Errorf("expect error key's aggregation contain %s", tc.info[i])
				}
			}
			for i := range tc.keys {
				ek.del(tc.keys[i])
			}
			if ek.length() != 0 {
				t.Errorf("expect length %v, got %v", tc.length, ek.length())
			}
			ek.queue.ShutDown()
		})
	}
}

func TestRecover(t *testing.T) {
	op := operation{
		Key:      "kubelet",
		Val:      "fail to xxx",
		Operator: PUT,
	}
	data, err := json.Marshal(op)
	if err != nil {
		t.Errorf("failed to marshal: %v", err)
	}
	aofPath, err := os.MkdirTemp("", "errorkeys")
	if err != nil {
		t.Errorf("failed to create dir: %v", err)
	}
	defer os.RemoveAll(aofPath)
	file, err := os.OpenFile(filepath.Join(aofPath, "aof"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Errorf("failed to open file: %v", err)
	}
	file.Write(data)
	file.Sync()
	file.Close()
	ek := NewErrorKeys(aofPath)
	ek.recover()
	if _, ok := ek.keys[op.Key]; !ok {
		t.Errorf("failed to recover")
	}
	ek.queue.ShutDown()
}

func TestCompress(t *testing.T) {
	aofPath, err := os.MkdirTemp("", "errorkeys")
	if err != nil {
		t.Errorf("failed to create dir: %v", err)
	}
	defer os.RemoveAll(aofPath)
	keys := NewErrorKeys(aofPath)
	for i := 0; i < 50; i++ {
		keys.put(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}
	for i := 0; i < 25; i++ {
		keys.del(fmt.Sprintf("key-%d", i))
	}
	for i := 0; i < 25; i++ {
		keys.put(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}
	err = wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, false,
		func(ctx context.Context) (bool, error) {
			if keys.errorCount() == 50 {
				return true, nil
			}
			return false, nil
		})
	if err != nil {
		t.Errorf("failed to sync")
	}
}

// TestErrorKeysConcurrentAccessRace drives put()/del() (which feed the
// sync() goroutine started by NewErrorKeys) concurrently with direct calls
// to rewrite() and with a reader that repeats compress()'s threshold check.
// This is the same access pattern compress()'s 30-second ticker eventually
// produces in production; compress() itself isn't invoked here because its
// hardcoded ticker makes it impractical to trigger from a fast unit test.
//
// Before the fix, sync() wrote ek.count/ek.file with no lock at all while
// compress()'s check read them unlocked and rewrite() reassigned them under
// only a read lock, so this test reliably tripped `go test -race`.
func TestErrorKeysConcurrentAccessRace(t *testing.T) {
	aofPath, err := os.MkdirTemp("", "errorkeys")
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	defer os.RemoveAll(aofPath)

	ek := NewErrorKeys(aofPath)
	defer ek.queue.ShutDown()

	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		for i := 0; i < 500; i++ {
			key := fmt.Sprintf("key-%d", i)
			ek.put(key, "value")
			ek.del(key)
		}
	}()

	go func() {
		defer workers.Done()
		for i := 0; i < 5; i++ {
			ek.rewrite()
		}
	}()

	stop := make(chan struct{})
	var reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = ek.errorCount() > ek.length()+CompressThresh
			}
		}
	}()

	workers.Wait()
	close(stop)
	reader.Wait()
}
