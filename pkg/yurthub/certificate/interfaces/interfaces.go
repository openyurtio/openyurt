package interfaces

import (
	"github.com/alibaba/openyurt/cmd/yurthub/app/config"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/certificate"
)

// YurtCertificateManager is responsible for managing node certificate for yurthub
type YurtCertificateManager interface {
	certificate.Manager
	Update(cfg *config.YurtHubConfiguration) error
	GetRestConfig() *rest.Config
	GetCaFile() string
	NotExpired() bool
}
