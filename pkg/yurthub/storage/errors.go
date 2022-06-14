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

package storage

import "errors"

// ErrStorageAccessConflict is an error for accessing key conflict
var ErrStorageAccessConflict = errors.New("specified key is under accessing")

// ErrStorageNotFound is an error for not found accessing key
var ErrStorageNotFound = errors.New("specified key is not found")

// ErrKeyHasNoContent is an error for file key that has no contents
var ErrKeyHasNoContent = errors.New("specified key has no contents")

// ErrKeyIsEmpty is an error for key is empty
var ErrKeyIsEmpty = errors.New("specified key is empty")

// ErrRootKeyInvalid is an error for root key is invalid
var ErrRootKeyInvalid = errors.New("root key is invalid")

// ErrKeyExists indicates that this key has already existed
var ErrKeyExists = errors.New("specified key has already existed")

// ErrUpdateConflict indicates that using an old object to update a new object
var ErrUpdateConflict = errors.New("update conflict for old resource version")

// ErrUnrecognizedKey indicates that this key cannot be recognized by this store
var ErrUnrecognizedKey = errors.New("unrecognized key")

// ErrEmptyComponent indicates that the component is empty.
var ErrEmptyComponent = errors.New("component is empty")

// ErrEmptyResource indicates that the resource is empty.
var ErrEmptyResource = errors.New("resource is empty")
