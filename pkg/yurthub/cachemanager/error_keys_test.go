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
			ek := NewErrorKeys()
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
			os.RemoveAll(AOFPrefix)
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
	err = os.MkdirAll(AOFPrefix, 0755)
	if err != nil {
		t.Errorf("failed to create dir: %v", err)
	}
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Errorf("failed to open file: %v", err)
	}
	file.Write(data)
	file.Sync()
	file.Close()
	ek := NewErrorKeys()
	ek.recover()
	if _, ok := ek.keys[op.Key]; !ok {
		t.Errorf("failed to recover")
	}
	ek.queue.ShutDown()
	os.RemoveAll(AOFPrefix)
}

func TestCompress(t *testing.T) {
	os.MkdirAll(AOFPrefix, 0644)
	keys := NewErrorKeys()
	for i := 0; i < 50; i++ {
		keys.put(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}
	for i := 0; i < 25; i++ {
		keys.del(fmt.Sprintf("key-%d", i))
	}
	for i := 0; i < 25; i++ {
		keys.put(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
	}
	err := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, false,
		func(ctx context.Context) (bool, error) {
			if keys.count == 50 {
				return true, nil
			}
			return false, nil
		})
	if err != nil {
		t.Errorf("failed to sync")
	}
}
