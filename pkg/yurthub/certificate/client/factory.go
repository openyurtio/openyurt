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

package client

import (
	"net/url"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/client/kubeletcertificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate/client/token"
)

const (
	KubeConfFile   = "/etc/kubernetes/kubelet.conf"
	KubeletPemFile = "/var/lib/kubelet/pki/kubelet-client-current.pem"
)

func NewYurtClientCertificateManager(options *options.YurtHubOptions, remoteServers []*url.URL, caManager certificate.YurtCACertificateManager, workDir string) (certificate.YurtClientCertificateManager, error) {
	switch options.BootstrapMode {
	case certificate.KubeletCertificateBootstrapMode:
		return kubeletcertificate.NewKubeletCertManager(KubeConfFile, KubeletPemFile)
	case certificate.TokenBoostrapMode:
		fallthrough
	default:
		cfg := &token.ClientCertificateManagerConfiguration{
			WorkDir:                  workDir,
			NodeName:                 options.NodeName,
			JoinToken:                options.JoinToken,
			BootstrapFile:            options.BootstrapFile,
			YurtHubCertOrganizations: options.YurtHubCertOrganizations,
			RemoteServers:            remoteServers,
			Client:                   options.ClientForTest,
		}
		return token.NewYurtHubClientCertManager(cfg, caManager)
	}
}
