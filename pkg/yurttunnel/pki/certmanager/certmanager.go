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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
)

// NewCertManager creates a certificate manager that will generates a
// certificate by sending a csr to the apiserver
func NewCertManager(
	clientset kubernetes.Interface,
	organization, commonName string) (certificate.Manager, error) {
	return nil, errors.New("NOT IMPLEMENT YET")
}

// ApprApproveYurttunnelCSR approves the yurttunel related CSR
func ApproveYurttunnelCSR(clientset kubernetes.Interface) {
	panic("NOT IMPLEMENT YET")
}
