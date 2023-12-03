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

package certificate

import (
	"crypto/tls"
)

const (
	// KubeletCertificateBootstrapMode means that yurthub uses kubelet certificate
	// that located at /var/lib/kubelet/pki/current-kubelet.pem to bootstrap instead of
	// generating client certificates.
	KubeletCertificateBootstrapMode = "kubeletcertificate"

	// TokenBoostrapMode means that yurthub uses join token to create client certificates
	// and bootstrap itself.
	TokenBoostrapMode = "token"
)

// YurtCertificateManager is responsible for managing node certificate for yurthub
type YurtCertificateManager interface {
	YurtClientCertificateManager
	YurtServerCertificateManager
	// Ready should be called after yurt certificate manager started by Start.
	Ready() bool
}

// YurtClientCertificateManager is responsible for managing node client certificates for yurthub
type YurtClientCertificateManager interface {
	Start()
	Stop()
	UpdateBootstrapConf(joinToken string) error
	GetHubConfFile() string
	GetCAData() []byte
	GetCaFile() string
	GetAPIServerClientCert() *tls.Certificate
}

type YurtServerCertificateManager interface {
	Start()
	Stop()
	GetHubServerCert() *tls.Certificate
	GetHubServerCertFile() string
}
