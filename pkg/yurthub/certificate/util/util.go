package util

import (
	"fmt"
	"net/url"
	"os"

	"k8s.io/klog"

	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
)

// FileExists checks if specified file exists.
func FileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func LoadKubeletRestClientConfig(healthyServer *url.URL) (*rest.Config, error) {
	const (
		pairFile   = "/var/lib/kubelet/pki/kubelet-client-current.pem"
		rootCAFile = "/etc/kubernetes/pki/ca.crt"
	)

	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPool(rootCAFile); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", rootCAFile, err)
	} else {
		tlsClientConfig.CAFile = rootCAFile
	}

	if can, _ := certutil.CanReadCertAndKey(pairFile, pairFile); !can {
		return nil, fmt.Errorf("error reading %s, certificate and key must be supplied as a pair", pairFile)
	} else {
		tlsClientConfig.KeyFile = pairFile
		tlsClientConfig.CertFile = pairFile
	}

	return &rest.Config{
		Host:            healthyServer.String(),
		TLSClientConfig: tlsClientConfig,
	}, nil
}
