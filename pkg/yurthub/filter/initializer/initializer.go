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
	"k8s.io/client-go/kubernetes"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
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

// WantsMasterServiceAddr is an interface for setting mutated master service address
type WantsMasterServiceAddr interface {
	SetMasterServiceHost(host string) error
	SetMasterServicePort(port string) error
}

// WantsKubeClient is an interface for setting kube client
type WantsKubeClient interface {
	SetKubeClient(client kubernetes.Interface) error
}

// genericFilterInitializer is responsible for initializing generic filter
type genericFilterInitializer struct {
	factory           informers.SharedInformerFactory
	yurtFactory       yurtinformers.SharedInformerFactory
	nodeName          string
	nodePoolName      string
	masterServiceHost string
	masterServicePort string
	client            kubernetes.Interface
}

// New creates an filterInitializer object
func New(factory informers.SharedInformerFactory,
	yurtFactory yurtinformers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
	nodeName, nodePoolName, masterServiceHost, masterServicePort string) *genericFilterInitializer {
	return &genericFilterInitializer{
		factory:           factory,
		yurtFactory:       yurtFactory,
		nodeName:          nodeName,
		nodePoolName:      nodePoolName,
		masterServiceHost: masterServiceHost,
		masterServicePort: masterServicePort,
		client:            kubeClient,
	}
}

// Initialize used for executing filter initialization
func (fi *genericFilterInitializer) Initialize(ins filter.ObjectFilter) error {
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

	if wants, ok := ins.(WantsKubeClient); ok {
		if err := wants.SetKubeClient(fi.client); err != nil {
			return err
		}
	}

	return nil
}
