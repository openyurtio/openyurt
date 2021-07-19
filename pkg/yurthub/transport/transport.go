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

package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/interfaces"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

// Interface is an transport interface for managing clients that used to connecting kube-apiserver
type Interface interface {
	// concurrent use by multiple goroutines
	// CurrentTransport get transport that used by load balancer
	CurrentTransport() *http.Transport
	// close all net connections that specified by address
	Close(address string)
}

type transportManager struct {
	currentTransport *http.Transport
	certManager      interfaces.YurtCertificateManager
	closeAll         func()
	close            func(string)
	stopCh           <-chan struct{}
}

// NewTransportManager create an transport interface object.
func NewTransportManager(certMgr interfaces.YurtCertificateManager, stopCh <-chan struct{}) (Interface, error) {
	caFile := certMgr.GetCaFile()
	if len(caFile) == 0 {
		return nil, fmt.Errorf("ca cert file was not prepared when new tranport")
	}
	klog.V(2).Infof("use %s ca cert file to access remote server", caFile)

	cfg, err := tlsConfig(certMgr, caFile)
	if err != nil {
		klog.Errorf("could not get tls config when new transport, %v", err)
		return nil, err
	}

	d := util.NewDialer("transport manager")
	t := utilnet.SetTransportDefaults(&http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     cfg,
		MaxIdleConnsPerHost: 25,
		DialContext:         d.DialContext,
	})

	tm := &transportManager{
		currentTransport: t,
		certManager:      certMgr,
		closeAll:         d.CloseAll,
		close:            d.Close,
		stopCh:           stopCh,
	}
	tm.start()

	return tm, nil
}

func (tm *transportManager) CurrentTransport() *http.Transport {
	return tm.currentTransport
}

func (tm *transportManager) Close(address string) {
	tm.close(address)
}

func (tm *transportManager) start() {
	lastCert := tm.certManager.Current()

	go wait.Until(func() {
		curr := tm.certManager.Current()

		if lastCert == nil && curr == nil {
			// maybe at yurthub startup, just wait for cert generated, do nothing
		} else if lastCert == nil && curr != nil {
			// cert generated, close all client connections for load new cert
			klog.Infof("new cert generated, so close all client connections for loading new cert")
			tm.closeAll()
			lastCert = curr
		} else if lastCert != nil && curr != nil {
			if lastCert == curr {
				// cert is not rotate, just wait
			} else {
				// cert rotated
				klog.Infof("cert rotated, so close all client connections for loading new cert")
				tm.closeAll()
				lastCert = curr
			}
		} else {
			// lastCet != nil && curr == nil
			// certificate expired or deleted unintentionally, just wait for cert updated by bootstrap config, do nothing
		}
	}, 10*time.Second, tm.stopCh)
}

func tlsConfig(certMgr interfaces.YurtCertificateManager, caFile string) (*tls.Config, error) {
	root, err := rootCertPool(caFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
		RootCAs:    root,
	}

	tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		cert := certMgr.Current()
		if cert == nil {
			return &tls.Certificate{Certificate: nil}, nil
		}
		return cert, nil
	}

	return tlsConfig, nil
}

func rootCertPool(caFile string) (*x509.CertPool, error) {
	if len(caFile) > 0 {
		if caFileExists, err := util.FileExists(caFile); err != nil {
			return nil, err
		} else if caFileExists {
			caData, err := ioutil.ReadFile(caFile)
			if err != nil {
				return nil, err
			}

			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(caData)
			return certPool, nil
		}
	}

	return nil, fmt.Errorf("failed to load ca file(%s)", caFile)
}
