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

type Factory func(cfg *config.YurtHubConfiguration) (interfaces.YurtCertificateManager, error)

type CertificateManagerRegistry struct {
	sync.Mutex
	registry map[string]Factory
}

func NewCertificateManagerRegistry() *CertificateManagerRegistry {
	return &CertificateManagerRegistry{}
}

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
