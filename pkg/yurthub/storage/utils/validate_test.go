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

// mockValidKey implements storage.Key + storage.KeyFormatValidator.
type mockValidKey struct {
	path      string
	shouldErr bool
}

func (k mockValidKey) Key() string {
	return k.path
}

func (k mockValidKey) Validate() error {
	if k.shouldErr {
		return errors.New("bad format")
	}
	return nil
}

// mockPlainKey implements ONLY storage.Key (no Validate).
// It simulates ClusterInfoKey.
type mockPlainKey struct {
	path string
}

func (k mockPlainKey) Key() string {
	return k.path
}

func TestValidateKey(t *testing.T) {
	validType := mockValidKey{}
	plainType := mockPlainKey{}

	t.Run("nil key returns ErrKeyIsEmpty", func(t *testing.T) {
		err := ValidateKey(nil, validType)
		if !errors.Is(err, storage.ErrKeyIsEmpty) {
			t.Errorf("expected ErrKeyIsEmpty, got %v", err)
		}
	})

	t.Run("empty key string returns ErrKeyIsEmpty", func(t *testing.T) {
		err := ValidateKey(mockValidKey{path: ""}, validType)
		if !errors.Is(err, storage.ErrKeyIsEmpty) {
			t.Errorf("expected ErrKeyIsEmpty, got %v", err)
		}
	})

	t.Run("wrong concrete type returns ErrUnrecognizedKey", func(t *testing.T) {
		err := ValidateKey(mockPlainKey{path: "x"}, validType)
		if !errors.Is(err, storage.ErrUnrecognizedKey) {
			t.Errorf("expected ErrUnrecognizedKey, got %v", err)
		}
	})

	t.Run("valid key with good format passes", func(t *testing.T) {
		err := ValidateKey(
			mockValidKey{path: "x", shouldErr: false},
			validType,
		)
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("valid type but bad internal format is rejected", func(t *testing.T) {
		err := ValidateKey(
			mockValidKey{path: "x", shouldErr: true},
			validType,
		)
		if err == nil {
			t.Errorf("expected format validation error, got nil")
		}
	})

	t.Run("key type without Validate still passes", func(t *testing.T) {
		err := ValidateKey(mockPlainKey{path: "x"}, plainType)
		if err != nil {
			t.Errorf("expected nil (no Validate method), got %v", err)
		}
	})
}

func TestValidateKV(t *testing.T) {
	validKeyType := mockValidKey{}

	cases := map[string]struct {
		key          storage.Key
		content      []byte
		validKeyType interface{}
		expectedErr  error
	}{
		"nil key": {
			key:          nil,
			content:      []byte("data"),
			validKeyType: validKeyType,
			expectedErr:  storage.ErrKeyIsEmpty,
		},
		"empty key": {
			key:          mockValidKey{path: ""},
			content:      []byte("data"),
			validKeyType: validKeyType,
			expectedErr:  storage.ErrKeyIsEmpty,
		},
		"unrecognized key type": {
			key:          mockPlainKey{
				path: "kubelet/pods.v1.core/default/foo",
			},
			content:      []byte("data"),
			validKeyType: validKeyType,
			expectedErr:  storage.ErrUnrecognizedKey,
		},
		"invalid key format": {
			key: mockValidKey{
				path:      "kubelet/pods.v1.core/default/foo",
				shouldErr: true,
			},
			content:      []byte("data"),
			validKeyType: validKeyType,
			expectedErr:  errors.New("bad format"),
		},
		"empty content": {
			key: mockValidKey{
				path: "kubelet/pods.v1.core/default/foo",
			},
			content:      []byte{},
			validKeyType: validKeyType,
			expectedErr:  storage.ErrKeyHasNoContent,
		},
		"valid key and content": {
			key: mockValidKey{
				path: "kubelet/pods.v1.core/default/foo",
			},
			content:      []byte("data"),
			validKeyType: validKeyType,
			expectedErr:  nil,
		},
		"plain key without Validate and valid content": {
			key: mockPlainKey{
				path: "kubelet/pods.v1.core/default/foo",
			},
			content:      []byte("data"),
			validKeyType: mockPlainKey{},
			expectedErr:  nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateKV(tc.key, tc.content, tc.validKeyType)

			if tc.expectedErr != nil {
				if err == nil {
					t.Errorf(
						"ValidateKV() error = nil, want %v",
						tc.expectedErr,
					)
					return
				}

				if tc.expectedErr.Error() == "bad format" {
					if err.Error() != tc.expectedErr.Error() {
						t.Errorf(
							"ValidateKV() error = %v, want %v",
							err,
							tc.expectedErr,
						)
					}
					return
				}

				if !errors.Is(err, tc.expectedErr) {
					t.Errorf(
						"ValidateKV() error = %v, want %v",
						err,
						tc.expectedErr,
					)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateKV() error = %v, want nil", err)
			}
		})
	}
}