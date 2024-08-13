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

package kubeletcertificate

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewKubeletCertManager(t *testing.T) {
	testcases := map[string]struct {
		kubeConfFile   string
		kubeletCAFile  string
		kubeletPemFile string
		err            error
	}{
		"kubelet.conf doesn't exist": {
			kubeConfFile: "invalid file",
			err:          KubeConfNotExistErr,
		},
		"ca.crt file doesn't exist": {
			kubeConfFile:  "../testdata/kubelet.conf",
			kubeletCAFile: "invalid file",
			err:           KubeletCANotExistErr,
		},
		"kubelet.pem doesn't exist": {
			kubeConfFile:   "../testdata/kubelet.conf",
			kubeletCAFile:  "../testdata/ca.crt",
			kubeletPemFile: "invalid file",
			err:            KubeletPemNotExistErr,
		},
		"normal kubelet cert manager": {
			kubeConfFile:   "../testdata/kubelet.conf",
			kubeletCAFile:  "../testdata/ca.crt",
			kubeletPemFile: "../testdata/kubelet.pem",
			err:            nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			_, err := NewKubeletCertManager(tc.kubeConfFile, tc.kubeletCAFile, tc.kubeletPemFile)
			if err != tc.err {
				t.Errorf("expect error is %v, but got %v", tc.err, err)
			}
		})
	}
}

// TestStart tests the Start method of kubeletCertManager
func TestStart(t *testing.T) {
	kcm := &kubeletCertManager{}
	kcm.Start()
	// No assertion needed as Start method does nothing
}

// TestStop tests the Stop method of kubeletCertManager
func TestStop(t *testing.T) {
	kcm := &kubeletCertManager{}
	kcm.Stop()
	// No assertion needed as Stop method does nothing
}

// TestUpdateBootstrapConf tests the UpdateBootstrapConf method of kubeletCertManager
func TestUpdateBootstrapConf(t *testing.T) {
	kcm := &kubeletCertManager{}
	err := kcm.UpdateBootstrapConf("test")
	assert.NoError(t, err)
}

// TestGetHubConfFile tests the GetHubConfFile method of kubeletCertManager
func TestGetHubConfFile(t *testing.T) {
	expectedFile := "test.conf"
	kcm := &kubeletCertManager{kubeConfFile: expectedFile}
	file := kcm.GetHubConfFile()
	assert.Equal(t, expectedFile, file)
}

// TestGetCAData tests the GetCAData method of kubeletCertManager
func TestGetCAData(t *testing.T) {
	expectedData := []byte("test")
	kcm := &kubeletCertManager{caData: expectedData}
	data := kcm.GetCAData()
	assert.Equal(t, expectedData, data)
}

// TestGetCaFile tests the GetCaFile method of kubeletCertManager
func TestGetCaFile(t *testing.T) {
	expectedFile := "test.ca"
	kcm := &kubeletCertManager{kubeletCAFile: expectedFile}
	file := kcm.GetCaFile()
	assert.Equal(t, expectedFile, file)
}

// TestGetAPIServerClientCert tests the GetAPIServerClientCert method of kubeletCertManager
func TestGetAPIServerClientCert(t *testing.T) {
	kcm := &kubeletCertManager{
		kubeletPemFile: "test.pem",
		cert: &tls.Certificate{
			Leaf: &x509.Certificate{
				NotAfter: time.Now().Add(time.Hour),
			},
		},
	}
	cert := kcm.GetAPIServerClientCert()
	assert.NotNil(t, cert)
}
