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

// YurtCertificateManager is responsible for managing node certificate for yurthub
type YurtCertificateManager interface {
	Start()
	Stop()
	// Ready should be called after yurt certificate manager started by Start.
	Ready() bool
	UpdateBootstrapConf(joinToken string) error
	GetHubConfFile() string
	GetCaFile() string
	GetAPIServerClientCert() *tls.Certificate
	GetHubServerCert() *tls.Certificate
	GetHubServerCertFile() string
}
