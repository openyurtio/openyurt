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

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	certfactory "github.com/openyurtio/openyurt/pkg/util/certmanager/factory"
	"github.com/openyurtio/openyurt/pkg/util/certmanager/store"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	hubCert "github.com/openyurtio/openyurt/pkg/yurthub/certificate"
)

type hubServerCertificateManager struct {
	hubServerCertManager certificate.Manager
	hubServerCertStore   certificate.FileStore
	kubeSvcClusterIP     net.IP
}

func NewHubServerCertificateManager(client clientset.Interface, clientCertManager hubCert.YurtClientCertificateManager, nodeName, pkiDir string, certIPs []net.IP) (hubCert.YurtServerCertificateManager, error) {
	hubServerCertStore, err := store.NewFileStoreWrapper(fmt.Sprintf("%s-server", projectinfo.GetHubName()), pkiDir, pkiDir, "", "")
	if err != nil {
		return nil, errors.Wrap(err, "couldn't new hub server cert store")
	}

	hscm := &hubServerCertificateManager{
		hubServerCertStore: hubServerCertStore,
	}

	kubeClientFn := func(current *tls.Certificate) (clientset.Interface, error) {
		// waiting for the certificate is generated
		_ = wait.PollInfinite(5*time.Second, func() (bool, error) {
			// keep polling until the yurthub client certificate is signed
			if clientCertManager.GetAPIServerClientCert() != nil {
				return true, nil
			}
			klog.Infof("waiting for the controller-manager to sign the %s client certificate", projectinfo.GetHubName())
			return false, nil
		})

		if !yurtutil.IsNil(client) {
			return client, nil
		}

		return kubeconfigutil.ClientSetFromFile(clientCertManager.GetHubConfFile())
	}

	serverIPsGetter := func() ([]net.IP, error) {
		ips := []net.IP{}
		if hscm.kubeSvcClusterIP != nil {
			ips = append(ips, hscm.kubeSvcClusterIP)
			ips = append(ips, certIPs...)
			return ips, nil
		}
		if clientCertManager.GetAPIServerClientCert() == nil {
			return ips, fmt.Errorf("client certificate(%s) of hub agent is not ready", clientCertManager.GetHubConfFile())
		}

		if !yurtutil.IsNil(client) {
			return certIPs, nil
		}

		kubeClient, err := kubeconfigutil.ClientSetFromFile(clientCertManager.GetHubConfFile())
		if err != nil {
			return ips, err
		}

		kubeSvc, err := kubeClient.CoreV1().Services("default").Get(context.Background(), "kubernetes", metav1.GetOptions{})
		if err != nil {
			return ips, err
		} else if kubeSvc == nil {
			return ips, fmt.Errorf("couldn't get default/kubernetes service")
		} else {
			ip := net.ParseIP(kubeSvc.Spec.ClusterIP)
			if ip == nil {
				return ips, fmt.Errorf("couldn't get clusterIP from default/kubernetes service")
			}
			klog.Infof("completed to prepare default/kubernetes service clusterIP(%s) for server certificate", ip.String())
			hscm.kubeSvcClusterIP = ip
			ips = append(ips, ip)
		}

		ips = append(ips, certIPs...)

		return ips, nil
	}

	hubServerCertManager, sErr := certfactory.NewCertManagerFactoryWithFnAndStore(kubeClientFn, hubServerCertStore).New(&certfactory.CertManagerConfig{
		ComponentName:  fmt.Sprintf("%s-server", projectinfo.GetHubName()),
		SignerName:     certificatesv1.KubeletServingSignerName,
		ForServerUsage: true,
		CommonName:     fmt.Sprintf("system:node:%s", nodeName),
		Organizations:  []string{user.NodesGroup},
		IPGetter:       serverIPsGetter,
		DNSNames:       []string{"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local"},
	})
	if sErr != nil {
		return nil, sErr
	}

	hscm.hubServerCertManager = hubServerCertManager
	return hscm, nil
}

func (hcm *hubServerCertificateManager) Start() {
	hcm.hubServerCertManager.Start()
}

func (hcm *hubServerCertificateManager) Stop() {
	hcm.hubServerCertManager.Stop()
}

func (hcm *hubServerCertificateManager) GetHubServerCert() *tls.Certificate {
	return hcm.hubServerCertManager.Current()
}

func (hcm *hubServerCertificateManager) GetHubServerCertFile() string {
	return hcm.hubServerCertStore.CurrentPath()
}
