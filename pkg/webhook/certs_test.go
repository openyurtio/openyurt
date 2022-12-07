/*
Copyright 2023 The OpenYurt Authors.

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

package webhook

import (
	"testing"
)

//func TestGenerateCerts(serviceNamespace, serviceName string) *Certs {
func TestGenerateCerts(t *testing.T) {
	certs := GenerateCerts("kube-system", "yurt-conroller-manager-webhook")
	if len(certs.CACert) == 0 || len(certs.CAKey) == 0 || len(certs.Cert) == 0 || len(certs.Key) == 0 {
		t.Fail()
	}
}
