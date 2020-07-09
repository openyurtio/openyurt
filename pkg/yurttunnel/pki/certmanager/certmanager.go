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
	"errors"

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
)

// NewServerCertManager creates a certificate manager that will generates a
// certificate for the yurttunnel-server by sending a csr to the apiserver.
// This method will also runs a goroutine to approve all yurttunnel related csr
func NewServerCertManager(
	kubeClient clientset.Interface,
	certDir, name string,
	stopCh <-chan struct{}) (certificate.Manager, error) {
	return nil, errors.New("NOT IMPLEMENT YET")
}

// NewAgentCertManager creates a certificate manager that will generates a
// certificate for the yurttunnel-agent by sending a csr to the apiserver
func NewAgentCertManager(
	client clientset.Interface,
	name string) (certificate.Manager, error) {
	return nil, errors.New("NOT IMPLEMENT YET")
}
