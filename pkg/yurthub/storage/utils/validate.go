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
	"reflect"
	"strings"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

// ValidateKey validates a storage.Key is non-nil/non-empty and has the correct type.
func ValidateKey(key storage.Key, validKeyType interface{}) error {
	if key == nil || key.Key() == "" {
		return storage.ErrKeyIsEmpty
	}
	if reflect.TypeOf(key) != reflect.TypeOf(validKeyType) {
		return storage.ErrUnrecognizedKey
	}
	return nil
}

// ValidateDiskKey validates the internal format of a disk storage key.
// A valid disk key has the format: /<Component>/<Resource[.Version[.Group]]>/<Namespace>/<Name>
// or /<Component>/<Resource[.Version[.Group]]>/<Namespace> for list keys.
// Keys without a leading slash are accepted for backward compatibility.
func ValidateDiskKey(key storage.Key) error {
	if key == nil || key.Key() == "" {
		return storage.ErrKeyIsEmpty
	}
	path := key.Key()
	// Strip leading slash if present
	if strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
	}
	// Split into at most 3 parts: component, gvr, namespace/name
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return storage.ErrKeyIsEmpty
	}
	// Validate component (must not be empty)
	if parts[0] == "" {
		return storage.ErrKeyIsEmpty
	}
	// Validate GVR component: either a single resource name (non-enhancement mode)
	// or resource.version.group (enhancement mode)
	gvr := parts[1]
	gvrParts := strings.SplitN(gvr, ".", 3)
	switch len(gvrParts) {
	case 1:
		// Non-enhancement mode: just resource name
		if gvrParts[0] == "" {
			return storage.ErrKeyIsEmpty
		}
	case 3:
		// Enhancement mode: resource.version.group
		for _, p := range gvrParts {
			if p == "" {
				return storage.ErrKeyIsEmpty
			}
		}
	default:
		return storage.ErrKeyIsEmpty
	}
	// For object keys (non-root), validate namespace/name segment exists
	if len(parts) == 3 {
		nn := parts[2]
		if nn == "" {
			return storage.ErrKeyIsEmpty
		}
		nnParts := strings.SplitN(nn, "/", 2)
		for _, p := range nnParts {
			if p == "" {
				return storage.ErrKeyIsEmpty
			}
		}
	}
	return nil
}

func ValidateKV(key storage.Key, content []byte, validKeyType interface{}) error {
	if err := ValidateKey(key, validKeyType); err != nil {
		return err
	}
	if len(content) == 0 {
		return storage.ErrKeyHasNoContent
	}
	return nil
}
