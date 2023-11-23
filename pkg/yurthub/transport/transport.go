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
	"fmt"
	"net/http"
	"time"

	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util/certmanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type CertGetter interface {
	// GetAPIServerClientCert returns the currently selected certificate, as well as
	// the associated certificate and key data in PEM format.
	GetAPIServerClientCert() *tls.Certificate
	// GetCAData returns CA file data.
	GetCAData() []byte
}

// Interface is an transport interface for managing clients that used to connecting kube-apiserver
type Interface interface {
	// CurrentTransport get transport that used by load balancer
	// and can be used by multiple goroutines concurrently.
	CurrentTransport() http.RoundTripper
	// BearerTransport returns transport for proxying request with bearer token in header
	BearerTransport() http.RoundTripper
	// Close all net connections that specified by address
	Close(address string)
}

type transportManager struct {
	currentTransport *http.Transport
	bearerTransport  *http.Transport
	certGetter       CertGetter
	closeAll         func()
	close            func(string)
	stopCh           <-chan struct{}
}

// NewTransportManager create a transport interface object.
func NewTransportManager(certGetter CertGetter, stopCh <-chan struct{}) (Interface, error) {
	caData := certGetter.GetCAData()
	if len(caData) == 0 {
		return nil, fmt.Errorf("ca cert data was not prepared when new transport")
	}

	cfg, err := tlsConfig(certGetter.GetAPIServerClientCert, caData)
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

	bearerTLSCfg, err := tlsConfig(nil, caData)
	if err != nil {
		klog.Errorf("could not get tls config when new bearer transport, %v", err)
		return nil, err
	}

	bt := utilnet.SetTransportDefaults(&http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     bearerTLSCfg,
		MaxIdleConnsPerHost: 25,
		DialContext:         d.DialContext,
	})

	tm := &transportManager{
		currentTransport: t,
		bearerTransport:  bt,
		certGetter:       certGetter,
		closeAll:         d.CloseAll,
		close:            d.Close,
		stopCh:           stopCh,
	}
	tm.start()

	return tm, nil
}

func (tm *transportManager) CurrentTransport() http.RoundTripper {
	return tm.currentTransport
}

func (tm *transportManager) BearerTransport() http.RoundTripper {
	return tm.bearerTransport
}

func (tm *transportManager) Close(address string) {
	tm.close(address)
}

func (tm *transportManager) start() {
	lastCert := tm.certGetter.GetAPIServerClientCert()

	go wait.Until(func() {
		curr := tm.certGetter.GetAPIServerClientCert()

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
				klog.V(5).Infof("cert is not rotate, just wait")
			} else {
				// cert rotated
				klog.Infof("cert rotated, so close all client connections for loading new cert")
				tm.closeAll()
				lastCert = curr
			}
		} else {
			// lastCet != nil && curr == nil
			// certificate expired or deleted unintentionally, just wait for cert updated by bootstrap config, do nothing
			klog.Warningf("certificate expired or deleted unintentionally")
		}
	}, 10*time.Second, tm.stopCh)
}

func tlsConfig(current func() *tls.Certificate, caData []byte) (*tls.Config, error) {
	// generate the TLS configuration based on the latest certificate
	rootCert, err := certmanager.GenCertPoolUseCAData(caData)
	if err != nil {
		klog.Errorf("could not generate a x509 CertPool based on the given CA file, %v", err)
		return nil, err
	}
	tlsCfg, err := certmanager.GenTLSConfigUseCurrentCertAndCertPool(current, rootCert, "client")
	if err != nil {
		return nil, err
	}

	return tlsCfg, nil
}
