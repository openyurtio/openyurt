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

package initializer

import (
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
)

// WantsHealthChecker is an interface for setting health checker
type WantsHealthChecker interface {
	SetHealthChecker(checker healthchecker.HealthChecker)
}

// CertificateManagerInitializer is responsible for initializing certificate manager
type CertificateManagerInitializer struct {
	checker healthchecker.HealthChecker
}

// NewCMInitializer creates an *CertificateManagerInitializer object
func NewCMInitializer(checker healthchecker.HealthChecker) *CertificateManagerInitializer {
	return &CertificateManagerInitializer{
		checker: checker,
	}
}

// Initialize used for executing certificate manager initialization
func (cmi *CertificateManagerInitializer) Initialize(cm interfaces.YurtCertificateManager) {
	if wants, ok := cm.(WantsHealthChecker); ok {
		wants.SetHealthChecker(cmi.checker)
	}
}
