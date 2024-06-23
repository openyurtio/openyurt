/*
Copyright 2020 The OpenYurt Authors.

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

import (
	"k8s.io/apimachinery/pkg/runtime"
)

type ClusterInfoKey struct {
	ClusterInfoType
	UrlPath string
}

type ClusterInfoType string

const (
	Version          ClusterInfoType = "version"
	APIsInfo         ClusterInfoType = "apis"
	APIResourcesInfo ClusterInfoType = "api-resources"
	Unknown          ClusterInfoType = "unknown"
)

// Store is an interface for caching data into store
type Store interface {
	// Name will return the name of this store.
	Name() string

	clusterInfoHandler

	ListKeys(key Key) ([]Key, error)

	Replace(key Key, objs map[Key]runtime.Object) error

	Create(key Key, obj runtime.Object) error

	Delete(key Key) error

	Get(key Key) (runtime.Object, error)

	List(key Key) ([]runtime.Object, error)

	Update(key Key, obj runtime.Object, rv uint64) (runtime.Object, error)

	KeyFunc(info KeyBuildInfo) (Key, error)
}

// clusterInfoHandler contains functions for manipulating cluster info cache in the storage.
type clusterInfoHandler interface {
	// SaveClusterInfo will save content of cluster info into storage.
	// If the content has already existed in the storage, it will be overwritten with content.
	SaveClusterInfo(key ClusterInfoKey, content []byte) error
	// GetClusterInfo will get the cluster info of clusterInfoType from storage.
	// If the cluster info is not found in the storage, return ErrStorageNotFound.
	GetClusterInfo(key ClusterInfoKey) ([]byte, error)
}
