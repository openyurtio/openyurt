/*
Copyright 2021 The OpenYurt Authors.

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

package initializer

import (
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	"k8s.io/client-go/informers"
)

// WantsSharedInformerFactory is an interface for setting SharedInformerFactory
type WantsSharedInformerFactory interface {
	SetSharedInformerFactory(factory informers.SharedInformerFactory) error
}

// WantsYurtSharedInformerFactory is an interface for setting Yurt-App-Manager SharedInformerFactory
type WantsYurtSharedInformerFactory interface {
	SetYurtSharedInformerFactory(yurtFactory yurtinformers.SharedInformerFactory) error
}

// WantsNodeName is an interface for setting node name
type WantsNodeName interface {
	SetNodeName(nodeName string) error
}

// WantsNodeName is an interface for setting node name
type WantsSerializerManager interface {
	SetSerializerManager(s *serializer.SerializerManager) error
}

// WantsStorageWrapper is an interface for setting StorageWrapper
type WantsStorageWrapper interface {
	SetStorageWrapper(s cachemanager.StorageWrapper) error
}

// WantsMasterServiceAddr is an interface for setting mutated master service address
type WantsMasterServiceAddr interface {
	SetMasterServiceAddr(addr string) error
}

// genericFilterInitializer is responsible for initializing generic filter
type genericFilterInitializer struct {
	factory           informers.SharedInformerFactory
	yurtFactory       yurtinformers.SharedInformerFactory
	serializerManager *serializer.SerializerManager
	storageWrapper    cachemanager.StorageWrapper
	nodeName          string
	masterServiceAddr string
}

// New creates an filterInitializer object
func New(factory informers.SharedInformerFactory,
	yurtFactory yurtinformers.SharedInformerFactory,
	sm *serializer.SerializerManager,
	sw cachemanager.StorageWrapper,
	nodeName string,
	masterServiceAddr string) *genericFilterInitializer {
	return &genericFilterInitializer{
		factory:           factory,
		yurtFactory:       yurtFactory,
		serializerManager: sm,
		storageWrapper:    sw,
		nodeName:          nodeName,
		masterServiceAddr: masterServiceAddr,
	}
}

// Initialize used for executing filter initialization
func (fi *genericFilterInitializer) Initialize(ins filter.Interface) error {
	if wants, ok := ins.(WantsNodeName); ok {
		if err := wants.SetNodeName(fi.nodeName); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsMasterServiceAddr); ok {
		if err := wants.SetMasterServiceAddr(fi.masterServiceAddr); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsSharedInformerFactory); ok {
		if err := wants.SetSharedInformerFactory(fi.factory); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsYurtSharedInformerFactory); ok {
		if err := wants.SetYurtSharedInformerFactory(fi.yurtFactory); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsSerializerManager); ok {
		if err := wants.SetSerializerManager(fi.serializerManager); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsStorageWrapper); ok {
		if err := wants.SetStorageWrapper(fi.storageWrapper); err != nil {
			return err
		}
	}

	return nil
}
