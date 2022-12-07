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

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

// TODO: should also valid the key format
func ValidateKey(key storage.Key, validKeyType interface{}) error {
	if key == nil || key.Key() == "" {
		return storage.ErrKeyIsEmpty
	}
	if reflect.TypeOf(key) != reflect.TypeOf(validKeyType) {
		return storage.ErrUnrecognizedKey
	}
	return nil
}

func ValidateKV(key storage.Key, content []byte, valideKeyType interface{}) error {
	if err := ValidateKey(key, valideKeyType); err != nil {
		return err
	}
	if len(content) == 0 {
		return storage.ErrKeyHasNoContent
	}
	return nil
}
