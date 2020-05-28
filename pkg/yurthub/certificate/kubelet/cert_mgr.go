package kubelet

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/alibaba/openyurt/pkg/yurthub/healthchecker"

	"github.com/alibaba/openyurt/cmd/yurthub/app/config"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate"
	"github.com/alibaba/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/alibaba/openyurt/pkg/yurthub/util"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

const (
	certificateManagerName = "kubelet"
	defaultPairDir         = "/var/lib/kubelet/pki"
	defaultPairFile        = "kubelet-client-current.pem"
	defaultCaFile          = "/etc/kubernetes/pki/ca.crt"
	certVerifyDuration     = 30 * time.Minute
)

// Register registers a YurtCertificateManager
func Register(cmr *certificate.CertificateManagerRegistry) {
	cmr.Register(certificateManagerName, func(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error) {
		return NewKubeletCertManager(cfg, 0, "")
	})
}

type kubeletCertManager struct {
	certAccessLock     sync.RWMutex
	pairFile           string
	cert               *tls.Certificate
	stopCh             chan struct{}
	remoteServers      []*url.URL
	caFile             string
	certVerifyDuration time.Duration
	checker            healthchecker.HealthChecker
	stopped            bool
}

// NewKubeletCertManager creates a YurtCertificateManager
func NewKubeletCertManager(cfg *config.YurtHubConfiguration, period time.Duration, certDir string) (interfaces.YurtCertificateManager, error) {
	var cert *tls.Certificate
	var pairFile string
	if cfg == nil || len(cfg.RemoteServers) == 0 {
		return nil, fmt.Errorf("yurthub configuration is invalid")
	}

	if period == time.Duration(0) {
		period = certVerifyDuration
	}

	if len(certDir) == 0 {
		certDir = defaultPairDir
	}
	pairFile = filepath.Join(certDir, defaultPairFile)

	if pairFileExists, err := util.FileExists(pairFile); err != nil {
		return nil, err
	} else if pairFileExists {
		klog.Infof("Loading cert/key pair from %q.", pairFile)
		cert, err = loadFile(pairFile)
		if err != nil {
			return nil, err
		}
	}

	return &kubeletCertManager{
		pairFile:           pairFile,
		cert:               cert,
		remoteServers:      cfg.RemoteServers,
		caFile:             defaultCaFile,
		certVerifyDuration: period,
		stopCh:             make(chan struct{}),
	}, nil
}

// SetHealthChecker set healthChecker
func (kcm *kubeletCertManager) SetHealthChecker(checker healthchecker.HealthChecker) {
	kcm.checker = checker
}

// Stop stop cert manager
func (kcm *kubeletCertManager) Stop() {
	kcm.certAccessLock.Lock()
	defer kcm.certAccessLock.Unlock()
	if kcm.stopped {
		return
	}
	close(kcm.stopCh)
	kcm.stopped = true
}

// Start runs certificate reloading after rotation
func (kcm *kubeletCertManager) Start() {
	go wait.Until(func() {
		newCert, err := loadFile(kcm.pairFile)
		if err != nil {
			klog.Errorf("failed to load cert file %s, %v", kcm.pairFile, err)
			return
		}

		certChanged := true
		kcm.certAccessLock.RLock()
		if kcm.cert.Leaf.NotAfter.Equal(newCert.Leaf.NotAfter) {
			certChanged = false
		}
		kcm.certAccessLock.RUnlock()

		if certChanged {
			klog.Infof("cert file %s is updated", kcm.pairFile)
			kcm.updateCert(newCert)
		}

	}, kcm.certVerifyDuration, kcm.stopCh)
}

// Current get the current certificate
func (kcm *kubeletCertManager) Current() *tls.Certificate {
	kcm.certAccessLock.RLock()
	defer kcm.certAccessLock.RUnlock()
	return kcm.cert
}

// ServerHealthy always returns true
func (kcm *kubeletCertManager) ServerHealthy() bool {
	return true
}

// GetCaFile get an ca file
func (kcm *kubeletCertManager) GetCaFile() string {
	return kcm.caFile
}

// GetRestConfig get *rest.Config from kubelet.conf
func (kcm *kubeletCertManager) GetRestConfig() *rest.Config {
	var s *url.URL
	for _, server := range kcm.remoteServers {
		if kcm.checker.IsHealthy(server) {
			s = server
		}
	}
	if s == nil {
		return nil
	}

	cfg, err := util.LoadKubeletRestClientConfig(s)
	if err != nil {
		klog.Errorf("could not load kubelet rest client config, %v", err)
		return nil
	}
	return cfg
}

func (kcm *kubeletCertManager) NotExpired() bool {
	kcm.certAccessLock.RLock()
	defer kcm.certAccessLock.RUnlock()
	if kcm.cert == nil || kcm.cert.Leaf == nil || time.Now().After(kcm.cert.Leaf.NotAfter) {
		klog.V(2).Infof("Current certificate is expired.")
		return false
	}
	return true
}

func (kcm *kubeletCertManager) updateCert(c *tls.Certificate) {
	kcm.certAccessLock.Lock()
	defer kcm.certAccessLock.Unlock()
	kcm.cert = c
}

// Update do nothing
func (kcm *kubeletCertManager) Update(cfg *config.YurtHubConfiguration) error {
	return nil
}

func loadFile(pairFile string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(pairFile, pairFile)
	if err != nil {
		return nil, fmt.Errorf("could not convert data from %q into cert/key pair: %v", pairFile, err)
	}

	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse certificate data: %v", err)
	}
	cert.Leaf = certs[0]
	return &cert, nil
}
