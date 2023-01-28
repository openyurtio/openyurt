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
	"fmt"
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

func ExtractKeyBuildInfo(key storage.Key) (*storage.KeyBuildInfo, error) {
	storageKey, ok := key.(storageKey)
	if !ok {
		return nil, storage.ErrUnrecognizedKey
	}

	if storageKey.isRootKey() {
		return nil, fmt.Errorf("cannot extract KeyBuildInfo from disk key %s, root key is unsupported", key.Key())
	}

	path := strings.TrimPrefix(key.Key(), "/")
	elems := strings.SplitN(path, "/", 3)
	if len(elems) < 3 {
		return nil, fmt.Errorf("cannot parse disk key %s, invalid format", key.Key())
	}

	comp, gvr, namespaceName := elems[0], elems[1], elems[2]
	buildInfo := &storage.KeyBuildInfo{
		Component: comp,
	}

	// parse GVR
	gvrElems := strings.SplitN(gvr, ".", 3)
	switch len(gvrElems) {
	case 1:
		// DiskStorage does not run in enhancement mode.
		// For the purpose of backward compatibility.
		buildInfo.Resources = gvrElems[0]
	case 3:
		buildInfo.Resources, buildInfo.Version, buildInfo.Group = gvrElems[0], gvrElems[1], gvrElems[2]
	default:
		return nil, fmt.Errorf("cannot parse gvr of disk key %s, invalid format", key.Key())
	}

	// parse Namespace and Name
	nnElems := strings.SplitN(namespaceName, "/", 2)
	switch len(nnElems) {
	case 1:
		// non-namespaced object
		buildInfo.Name = nnElems[0]
	case 2:
		// namespaced object
		buildInfo.Namespace, buildInfo.Name = nnElems[0], nnElems[1]
	default:
		return nil, fmt.Errorf("cannot parse namespace name of disk key %s, invalid format", key.Key())
	}

	if buildInfo.Group == "core" {
		buildInfo.Group = ""
	}

	return buildInfo, nil
}
