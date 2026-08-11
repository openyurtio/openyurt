/*
Copyright 2022 The OpenYurt Authors.

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

package utils

import (
	"errors"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

type testKey struct {
	path string
}

func (k testKey) Key() string {
	return k.path
}

func TestValidateKey(t *testing.T) {
	cases := map[string]struct {
		key          storage.Key
		validKeyType interface{}
		expectedErr  error
	}{
		"nil key": {
			key:          nil,
			validKeyType: testKey{},
			expectedErr:  storage.ErrKeyIsEmpty,
		},
		"empty key": {
			key:          testKey{path: ""},
			validKeyType: testKey{},
			expectedErr:  storage.ErrKeyIsEmpty,
		},
		"unrecognized key type": {
			key:          testKey{path: "kubelet/pods.v1.core/default/foo"},
			validKeyType: storage.ClusterInfoKey{},
			expectedErr:  storage.ErrUnrecognizedKey,
		},
		"valid key": {
			key:          testKey{path: "kubelet/pods.v1.core/default/foo"},
			validKeyType: testKey{},
			expectedErr:  nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateKey(tc.key, tc.validKeyType)
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("ValidateKey() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

func TestValidateKV(t *testing.T) {
	cases := map[string]struct {
		key          storage.Key
		content      []byte
		validKeyType interface{}
		expectedErr  error
	}{
		"nil key": {
			key:          nil,
			content:      []byte("data"),
			validKeyType: testKey{},
			expectedErr:  storage.ErrKeyIsEmpty,
		},
		"unrecognized key type": {
			key:          testKey{path: "kubelet/pods.v1.core/default/foo"},
			content:      []byte("data"),
			validKeyType: storage.ClusterInfoKey{},
			expectedErr:  storage.ErrUnrecognizedKey,
		},
		"empty content": {
			key:          testKey{path: "kubelet/pods.v1.core/default/foo"},
			content:      []byte{},
			validKeyType: testKey{},
			expectedErr:  storage.ErrKeyHasNoContent,
		},
		"valid key and content": {
			key:          testKey{path: "kubelet/pods.v1.core/default/foo"},
			content:      []byte("data"),
			validKeyType: testKey{},
			expectedErr:  nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateKV(tc.key, tc.content, tc.validKeyType)
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("ValidateKV() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}
