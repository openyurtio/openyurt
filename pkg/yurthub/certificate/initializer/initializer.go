package initializer

import (
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"
)

type WantsHealthChecker interface {
	SetHealthChecker(checker healthchecker.HealthChecker)
}

type CertificateManagerInitializer struct {
	checker healthchecker.HealthChecker
}

func NewCMInitializer(checker healthchecker.HealthChecker) *CertificateManagerInitializer {
	return &CertificateManagerInitializer{
		checker: checker,
	}
}

func (cmi *CertificateManagerInitializer) Initialize(cm interfaces.YurtCertificateManager) {
	if wants, ok := cm.(WantsHealthChecker); ok {
		wants.SetHealthChecker(cmi.checker)
	}
}
