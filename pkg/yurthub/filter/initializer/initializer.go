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
	"k8s.io/client-go/informers"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
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

// WantsNodePoolName is an interface for setting nodePool name
type WantsNodePoolName interface {
	SetNodePoolName(nodePoolName string) error
}

// WantsStorageWrapper is an interface for setting StorageWrapper
type WantsStorageWrapper interface {
	SetStorageWrapper(s cachemanager.StorageWrapper) error
}

// WantsMasterServiceAddr is an interface for setting mutated master service address
type WantsMasterServiceAddr interface {
	SetMasterServiceHost(host string) error
	SetMasterServicePort(port string) error
}

// WantsWorkingMode is an interface for setting working mode
type WantsWorkingMode interface {
	SetWorkingMode(mode util.WorkingMode) error
}

// genericFilterInitializer is responsible for initializing generic filter
type genericFilterInitializer struct {
	factory           informers.SharedInformerFactory
	yurtFactory       yurtinformers.SharedInformerFactory
	storageWrapper    cachemanager.StorageWrapper
	nodeName          string
	nodePoolName      string
	masterServiceHost string
	masterServicePort string
	workingMode       util.WorkingMode
}

// New creates an filterInitializer object
func New(factory informers.SharedInformerFactory,
	yurtFactory yurtinformers.SharedInformerFactory,
	sw cachemanager.StorageWrapper,
	nodeName, nodePoolName, masterServiceHost, masterServicePort string,
	workingMode util.WorkingMode) *genericFilterInitializer {
	return &genericFilterInitializer{
		factory:           factory,
		yurtFactory:       yurtFactory,
		storageWrapper:    sw,
		nodeName:          nodeName,
		masterServiceHost: masterServiceHost,
		masterServicePort: masterServicePort,
		workingMode:       workingMode,
	}
}

// Initialize used for executing filter initialization
func (fi *genericFilterInitializer) Initialize(ins filter.ObjectFilter) error {
	if wants, ok := ins.(WantsWorkingMode); ok {
		if err := wants.SetWorkingMode(fi.workingMode); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsNodeName); ok {
		if err := wants.SetNodeName(fi.nodeName); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsNodePoolName); ok {
		if err := wants.SetNodePoolName(fi.nodePoolName); err != nil {
			return err
		}
	}

	if wants, ok := ins.(WantsMasterServiceAddr); ok {
		if err := wants.SetMasterServiceHost(fi.masterServiceHost); err != nil {
			return err
		}

		if err := wants.SetMasterServicePort(fi.masterServicePort); err != nil {
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

	if wants, ok := ins.(WantsStorageWrapper); ok {
		if err := wants.SetStorageWrapper(fi.storageWrapper); err != nil {
			return err
		}
	}

	return nil
}
