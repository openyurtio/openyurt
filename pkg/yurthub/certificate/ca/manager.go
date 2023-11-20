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

package ca

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	clientset "k8s.io/client-go/kubernetes"
	certutil "k8s.io/client-go/util/cert"

	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	hubPkiDirName = "pki"
	hubCaFileName = "ca.crt"

	kubeletCAFile = "/etc/kubernetes/pki/ca.crt"
)

type manager struct {
	caData     []byte
	caFilePath string
}

func NewCAManager(client clientset.Interface, mode, workDir, bootstrapFile, joinToken string, remoteServers []*url.URL, caCertHashes []string) (certificate.YurtCACertificateManager, error) {
	caFilePath := filepath.Join(workDir, hubPkiDirName, hubCaFileName)
	if mode == certificate.KubeletCertificateBootstrapMode {
		caFilePath = kubeletCAFile
	}
	mgr := &manager{
		caFilePath: caFilePath,
	}

	if exist, err := util.FileExists(mgr.caFilePath); err != nil {
		return mgr, err
	} else if exist {
		// only reuse the ca file to prepare CA Data
		caData, err := os.ReadFile(mgr.caFilePath)
		if err != nil {
			return mgr, err
		}
		mgr.caData = caData
		return mgr, nil
	}

	switch mode {
	case certificate.KubeletCertificateBootstrapMode:
		return mgr, fmt.Errorf("could not prepare CA data from kubelet CA file %s", kubeletCAFile)
	case certificate.TokenBoostrapMode:
		fallthrough
	default:
		if len(bootstrapFile) != 0 {
			caData, err := ResolveCADataByBootstrapFile(bootstrapFile)
			if err != nil {
				return mgr, err
			}
			mgr.caData = caData
		} else {
			caData, err := ResolveCADataByToken(client, remoteServers, joinToken, caCertHashes)
			if err != nil {
				return mgr, err
			}
			mgr.caData = caData
		}
	}

	// write ca data into ca file
	if err := certutil.WriteCert(mgr.caFilePath, mgr.caData); err != nil {
		return mgr, errors.Wrap(err, "couldn't save the CA certificate to disk")
	}

	return mgr, nil
}

func (m *manager) GetCAData() []byte {
	return m.caData
}

func (m *manager) GetCaFile() string {
	return m.caFilePath
}
