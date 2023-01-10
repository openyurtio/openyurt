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

package certmanager

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/certificate"
)

// GenTLSConfigUseCurrentCertAndCertPool generates a TLS configuration
// using the given current certificate and x509 CertPool
func GenTLSConfigUseCurrentCertAndCertPool(
	current func() *tls.Certificate,
	root *x509.CertPool,
	mode string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
	}

	switch mode {
	case "server":
		tlsConfig.ClientCAs = root
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		if current != nil {
			tlsConfig.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert := current()
				if cert == nil {
					return &tls.Certificate{Certificate: nil}, nil
				}
				return cert, nil
			}
		}
	case "client":
		tlsConfig.RootCAs = root
		if current != nil {
			tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				cert := current()
				if cert == nil {
					return &tls.Certificate{Certificate: nil}, nil
				}
				return cert, nil
			}
		}
	default:
		return nil, fmt.Errorf("unsupported cert manager mode(only server or client), %s", mode)
	}

	return tlsConfig, nil
}

// GenRootCertPool generates a x509 CertPool based on the given kubeconfig,
// if the kubeConfig is empty, it will creates the CertPool using the CA file
func GenRootCertPool(kubeConfig, caFile string) (*x509.CertPool, error) {
	if kubeConfig != "" {
		// kubeconfig is given, generate the clientset based on it
		if _, err := os.Stat(kubeConfig); os.IsNotExist(err) {
			return nil, err
		}

		// load the root ca from the given kubeconfig file
		config, err := clientcmd.LoadFromFile(kubeConfig)
		if err != nil || config == nil {
			return nil, fmt.Errorf("failed to load the kubeconfig file(%s), %w",
				kubeConfig, err)
		}

		if len(config.CurrentContext) == 0 {
			return nil, fmt.Errorf("'current context' is not set in %s",
				kubeConfig)
		}

		ctx, ok := config.Contexts[config.CurrentContext]
		if !ok || ctx == nil {
			return nil, fmt.Errorf("'current context(%s)' is not found in %s",
				config.CurrentContext, kubeConfig)
		}

		cluster, ok := config.Clusters[ctx.Cluster]
		if !ok || cluster == nil {
			return nil, fmt.Errorf("'cluster(%s)' is not found in %s",
				ctx.Cluster, kubeConfig)
		}

		if len(cluster.CertificateAuthorityData) == 0 {
			return nil, fmt.Errorf("'certificate authority data of the cluster(%s) is not set in %s",
				ctx.Cluster, kubeConfig)
		}

		rootCertPool := x509.NewCertPool()
		rootCertPool.AppendCertsFromPEM(cluster.CertificateAuthorityData)
		return rootCertPool, nil
	}

	// kubeConfig is missing, generate the cluster root ca based on the given ca file
	return GenCertPoolUseCA(caFile)
}

// GenTLSConfigUseCertMgrAndCA generates a TLS configuration based on the
// given certificate manager and the CA file
func GenTLSConfigUseCertMgrAndCA(
	m certificate.Manager,
	serverAddr, caFile string) (*tls.Config, error) {
	root, err := GenCertPoolUseCA(caFile)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
		ServerName: host,
		RootCAs:    root,
	}

	tlsConfig.GetClientCertificate =
		func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert := m.Current()
			if cert == nil {
				return &tls.Certificate{Certificate: nil}, nil
			}
			return cert, nil
		}
	tlsConfig.GetCertificate =
		func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert := m.Current()
			if cert == nil {
				return &tls.Certificate{Certificate: nil}, nil
			}
			return cert, nil
		}

	return tlsConfig, nil
}

// GenCertPoolUseCA generates a x509 CertPool based on the given CA file
func GenCertPoolUseCA(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, errors.New("CA file is not set")
	}

	if _, err := os.Stat(caFile); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("CA file(%s) doesn't exist", caFile)
		}
		return nil, fmt.Errorf("fail to stat the CA file(%s): %w", caFile, err)
	}

	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caData)
	return certPool, nil
}
