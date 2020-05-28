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
