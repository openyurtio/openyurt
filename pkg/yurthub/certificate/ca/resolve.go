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
	"net"
	"net/url"

	"github.com/pkg/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/util/token"
	yurtcertutil "github.com/openyurtio/openyurt/pkg/yurthub/certificate/util"
)

func ResolveCADataByToken(client clientset.Interface, remoteServers []*url.URL, joinToken string, caCertHashes []string) ([]byte, error) {
	// retrieve bootstrap config info from cluster-info configmap by bootstrap token
	serverAddr := yurtcertutil.FindActiveRemoteServer(net.DialTimeout, remoteServers).Host
	cfg, err := token.RetrieveValidatedConfigInfo(client, &token.BootstrapData{
		ServerAddr:   serverAddr,
		JoinToken:    joinToken,
		CaCertHashes: caCertHashes,
	})

	if err != nil {
		return []byte{}, errors.Wrap(err, "couldn't retrieve bootstrap config info")
	}

	clusterInfo := kubeconfigutil.GetClusterFromKubeConfig(cfg)
	if clusterInfo != nil && len(clusterInfo.CertificateAuthorityData) != 0 {
		return clusterInfo.CertificateAuthorityData, nil
	}

	return []byte{}, errors.New("could not get CA data")
}

func ResolveCADataByBootstrapFile(bootstrapFilePath string) ([]byte, error) {
	// prepare ca data and file based on the hub bootstrap file
	if tlsBootstrapCfg, err := clientcmd.LoadFromFile(bootstrapFilePath); err != nil {
		return []byte{}, err
	} else {
		cluster := kubeconfigutil.GetClusterFromKubeConfig(tlsBootstrapCfg)
		if cluster != nil && len(cluster.CertificateAuthorityData) != 0 {
			return cluster.CertificateAuthorityData, nil
		}
	}

	return []byte{}, fmt.Errorf("could not prepare CA data from bootstrp file %s", bootstrapFilePath)
}
