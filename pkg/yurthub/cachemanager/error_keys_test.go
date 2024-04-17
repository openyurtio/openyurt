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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			ek.cancel()
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
	os.MkdirAll(AOFPrefix, 0755)
	file, err := os.OpenFile(filepath.Join(AOFPrefix, "aof"), os.O_CREATE|os.O_RDWR, 0600)
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
	ek.cancel()
	os.RemoveAll(AOFPrefix)
}
