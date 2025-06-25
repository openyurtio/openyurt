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
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// TransportManager is an interface for managing clients that used to connecting kube-apiserver
type TransportManager interface {
	// CurrentTransport get transport that used by load balancer
	// and can be used by multiple goroutines concurrently.
	CurrentTransport() http.RoundTripper
	// BearerTransport returns transport for proxying request with bearer token in header
	BearerTransport() http.RoundTripper
	// Close all net connections that specified by address
	Close(address string)
	// GetDirectClientset returns clientset specified by url
	GetDirectClientset(url *url.URL) kubernetes.Interface
	// GetDirectClientsetAtRandom returns a clientset at random
	GetDirectClientsetAtRandom() kubernetes.Interface
	// ListDirectClientset returns all clientsets
	ListDirectClientset() map[string]kubernetes.Interface

	// Start manager internal work.
	Start(context.Context)
}

type transportAndClientManager struct {
	currentTransport  *http.Transport
	bearerTransport   *http.Transport
	certGetter        CertGetter
	closeAll          func()
	close             func(string)
	serverToClientset map[string]kubernetes.Interface
}

// NewTransportManager create a transport interface object.
func NewTransportAndClientManager(servers []*url.URL, timeout int, certGetter CertGetter) (TransportManager, error) {
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

	tcm := &transportAndClientManager{
		currentTransport:  t,
		bearerTransport:   bt,
		certGetter:        certGetter,
		closeAll:          d.CloseAll,
		close:             d.Close,
		serverToClientset: make(map[string]kubernetes.Interface),
	}

	for i := range servers {
		config := &rest.Config{
			Host:      servers[i].String(),
			Transport: t,
			Timeout:   time.Duration(timeout) * time.Second,
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		if len(servers[i].String()) != 0 {
			tcm.serverToClientset[servers[i].String()] = clientset
		}
	}

	return tcm, nil
}

func (tcm *transportAndClientManager) CurrentTransport() http.RoundTripper {
	return tcm.currentTransport
}

func (tcm *transportAndClientManager) BearerTransport() http.RoundTripper {
	return tcm.bearerTransport
}

func (tcm *transportAndClientManager) Close(address string) {
	tcm.close(address)
}

func (tcm *transportAndClientManager) GetDirectClientset(url *url.URL) kubernetes.Interface {
	if url != nil {
		return tcm.serverToClientset[url.String()]
	}
	return nil
}

func (tcm *transportAndClientManager) GetDirectClientsetAtRandom() kubernetes.Interface {
	// iterating map uses random order
	for server := range tcm.serverToClientset {
		return tcm.serverToClientset[server]
	}

	return nil
}

func (tcm *transportAndClientManager) ListDirectClientset() map[string]kubernetes.Interface {
	return tcm.serverToClientset
}

func (tcm *transportAndClientManager) Start(ctx context.Context) {
	lastCert := tcm.certGetter.GetAPIServerClientCert()

	go wait.Until(func() {
		curr := tcm.certGetter.GetAPIServerClientCert()

		if lastCert == nil && curr == nil {
			// maybe at yurthub startup, just wait for cert generated, do nothing
		} else if lastCert == nil && curr != nil {
			// cert generated, close all client connections for load new cert
			klog.Infof("new cert generated, so close all client connections for loading new cert")
			tcm.closeAll()
			lastCert = curr
		} else if lastCert != nil && curr != nil {
			if lastCert == curr {
				// cert is not rotate, just wait
				klog.V(5).Infof("cert is not rotate, just wait")
			} else {
				// cert rotated
				klog.Infof("cert rotated, so close all client connections for loading new cert")
				tcm.closeAll()
				lastCert = curr
			}
		} else {
			// lastCet != nil && curr == nil
			// certificate expired or deleted unintentionally, just wait for cert updated by bootstrap config, do nothing
			klog.Warningf("certificate expired or deleted unintentionally")
		}
	}, 10*time.Second, ctx.Done())
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
