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

package certificate

import (
	"fmt"
	"sync"
	"time"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/initializer"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

// Factory is a function that returns an YurtCertificateManager.
// The cfg parameter provides the common info for certificate manager
type Factory func(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error)

// CertificateManagerRegistry is a object for holding all of certificate managers
type CertificateManagerRegistry struct {
	sync.Mutex
	registry map[string]Factory
}

// NewCertificateManagerRegistry creates an *CertificateManagerRegistry object
func NewCertificateManagerRegistry() *CertificateManagerRegistry {
	return &CertificateManagerRegistry{}
}

// Register register a Factory func for creating certificate manager
func (cmr *CertificateManagerRegistry) Register(name string, cm Factory) {
	cmr.Lock()
	defer cmr.Unlock()

	if cmr.registry == nil {
		cmr.registry = map[string]Factory{}
	}

	_, found := cmr.registry[name]
	if found {
		klog.Fatalf("certificate manager %s was registered twice", name)
	}

	klog.Infof("Registered certificate manager %s", name)
	cmr.registry[name] = cm
}

// New creates a YurtCertificateManager with specified name of registered certificate manager
func (cmr *CertificateManagerRegistry) New(name string, cfg *config.YurtHubConfiguration, cmInitializer *initializer.CertificateManagerInitializer) (interfaces.YurtCertificateManager, error) {
	f, found := cmr.registry[name]
	if !found {
		return nil, fmt.Errorf("certificate manager %s is not registered", name)
	}

	cm, err := f(cfg)
	if err != nil {
		return nil, err
	}

	cmInitializer.Initialize(cm)

	cm.Start()
	err = wait.PollImmediate(5*time.Second, 4*time.Minute, func() (bool, error) {
		curr := cm.Current()
		if curr != nil {
			return true, nil
		}

		klog.Infof("waiting for preparing client certificate")
		return false, nil
	})
	if err != nil {
		klog.Errorf("client certificate preparation failed, %v", err)
		return nil, err
	}

	return cm, nil
}
