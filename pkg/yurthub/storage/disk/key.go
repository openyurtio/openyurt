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

package disk

import (
	"path/filepath"
	"strings"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

type storageKey struct {
	rootKey bool
	path    string
}

func (k storageKey) Key() string {
	return k.path
}

func (k storageKey) isRootKey() bool {
	return k.rootKey
}

// Key for disk storage is
// /<Component>/<Resource.Version.Group>/<Namespace>/<Name>, or
// /<Component>/<Resource.Version.Group>/<Name>, if there's no namespace provided in info.
// /<Component>/<Resource.Version.Group>/<Namespace>, if there's no name provided in info.
// /<Component>/<Resource.Version.Group>, if there's no namespace and name provided in info.
// If diskStorage does not run in enhancement mode, it will use the prefix of key as:
// /<Component>/<Resource>/
func (ds *diskStorage) KeyFunc(info storage.KeyBuildInfo) (storage.Key, error) {
	isRoot := false
	if info.Component == "" {
		return nil, storage.ErrEmptyComponent
	}
	if info.Resources == "" {
		return nil, storage.ErrEmptyResource
	}
	if info.Name == "" {
		isRoot = true
	}

	group := info.Group
	if info.Group == "" {
		group = "core"
	}

	var path, resource string
	if ds.enhancementMode {
		resource = strings.Join([]string{info.Resources, info.Version, group}, ".")
	} else {
		resource = info.Resources
	}

	if info.Resources == "namespaces" {
		path = filepath.Join(info.Component, resource, info.Name)
	} else {
		path = filepath.Join(info.Component, resource, info.Namespace, info.Name)
	}

	return storageKey{
		path:    path,
		rootKey: isRoot,
	}, nil
}
