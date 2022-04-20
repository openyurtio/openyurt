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
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"os"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/util/certmanager/store"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/server/serveraddr"
)

const (
	YurtHubCSROrg         = "openyurt:yurthub"
	YurtTunnelCSROrg      = "openyurt:yurttunnel"
	YurtTunnelServerCSRCN = "tunnel-server"
	YurtTunnelAgentCSRCN  = "tunnel-agent-client"
	// use custom signer for k8s1.22, detail in https://kubernetes.io/docs/reference/access-authn-authz/certificate-signing-requests/
	YurtTunnelSignerName = "openyurt.io/yurt-tunnel"
)

// NewYurttunnelServerCertManager creates a certificate manager for
// the yurttunnel-server
func NewYurttunnelServerCertManager(
	clientset kubernetes.Interface,
	factory informers.SharedInformerFactory,
	certDir string,
	clCertNames []string,
	clIPs []net.IP,
	stopCh <-chan struct{}) (certificate.Manager, error) {
	var (
		dnsNames = []string{}
		ips      = []net.IP{}
		err      error
	)

	// the ips and dnsNames should be acquired through api-server at the first time, because the informer factory has not started yet.
	_ = wait.PollUntil(5*time.Second, func() (bool, error) {
		dnsNames, ips, err = serveraddr.GetYurttunelServerDNSandIP(clientset)
		if err != nil {
			klog.Errorf("failed to get yurt tunnel server dns and ip, %v", err)
			return false, err
		}

		// get clusterIP for tunnel server internal service
		svc, err := clientset.CoreV1().Services(constants.YurttunnelServerServiceNs).Get(context.Background(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			// compatible with versions that not supported x-tunnel-server-internal-svc
			klog.Warningf("get service: %s not found", constants.YurttunnelServerInternalServiceName)
			return true, nil
		} else if err != nil {
			klog.Warningf("get service: %s err, %v", constants.YurttunnelServerInternalServiceName, err)
			return false, err
		}

		if svc.Spec.ClusterIP != "" && net.ParseIP(svc.Spec.ClusterIP) != nil {
			ips = append(ips, net.ParseIP(svc.Spec.ClusterIP))
			dnsNames = append(dnsNames, serveraddr.GetDefaultDomainsForSvc(svc.Namespace, svc.Name)...)
		}

		return true, nil
	}, stopCh)
	// add user specified DNS names and IP addresses
	dnsNames = append(dnsNames, clCertNames...)
	ips = append(ips, clIPs...)
	klog.Infof("subject of tunnel server certificate, ips=%#+v, dnsNames=%#+v", ips, dnsNames)

	// the dynamic ip acquire func
	getIPs := func() ([]net.IP, error) {
		_, dynamicIPs, err := serveraddr.YurttunnelServerAddrManager(factory)
		dynamicIPs = append(dynamicIPs, clIPs...)
		return dynamicIPs, err
	}

	return newCertManager(
		clientset,
		projectinfo.GetServerName(),
		certDir,
		YurtTunnelSignerName,
		YurtTunnelServerCSRCN,
		[]string{user.SystemPrivilegedGroup, YurtTunnelCSROrg},
		dnsNames,
		[]certificatesv1.KeyUsage{
			certificatesv1.UsageKeyEncipherment,
			certificatesv1.UsageDigitalSignature,
			certificatesv1.UsageServerAuth,
			certificatesv1.UsageClientAuth,
		},
		ips,
		getIPs)
}

// NewYurttunnelAgentCertManager creates a certificate manager for
// the yurttunel-agent
func NewYurttunnelAgentCertManager(
	clientset kubernetes.Interface,
	certDir string) (certificate.Manager, error) {
	// As yurttunnel-agent will run on the edge node with Host network mode,
	// we can use the status.podIP as the node IP
	nodeIP := os.Getenv(constants.YurttunnelAgentPodIPEnv)
	if nodeIP == "" {
		return nil, fmt.Errorf("env %s is not set",
			constants.YurttunnelAgentPodIPEnv)
	}
	return newCertManager(
		clientset,
		projectinfo.GetAgentName(),
		certDir,
		YurtTunnelSignerName,
		YurtTunnelAgentCSRCN,
		[]string{YurtTunnelCSROrg},
		[]string{os.Getenv("NODE_NAME")},
		[]certificatesv1.KeyUsage{
			certificatesv1.UsageKeyEncipherment,
			certificatesv1.UsageDigitalSignature,
			certificatesv1.UsageClientAuth,
		},
		[]net.IP{net.ParseIP(nodeIP)},
		nil)
}

// NewYurtHubServerCertManager creates a certificate manager for
// the yurthub server
func NewYurtHubServerCertManager(
	clientset kubernetes.Interface,
	certDir,
	nodeName,
	proxyServerSecureDummyAddr string) (certificate.Manager, error) {

	host, _, err := net.SplitHostPort(proxyServerSecureDummyAddr)
	if err != nil {
		return nil, err
	}

	return newCertManager(
		clientset,
		fmt.Sprintf("%s-server", projectinfo.GetHubName()),
		certDir,
		certificatesv1.KubeletServingSignerName,
		fmt.Sprintf("system:node:%s", nodeName),
		[]string{user.NodesGroup},
		nil,
		[]certificatesv1.KeyUsage{
			certificatesv1.UsageKeyEncipherment,
			certificatesv1.UsageDigitalSignature,
			certificatesv1.UsageServerAuth,
		},
		[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP(host)},
		nil)
}

// NewCertManager creates a certificate manager that will generates a
// certificate by sending a csr to the apiserver
func newCertManager(
	clientset kubernetes.Interface,
	componentName,
	certDir,
	signerName,
	commonName string,
	organizations,
	dnsNames []string,
	keyUsages []certificatesv1.KeyUsage,
	ips []net.IP,
	getIPs serveraddr.GetIPs) (certificate.Manager, error) {
	certificateStore, err :=
		store.NewFileStoreWrapper(componentName, certDir, certDir, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the server certificate store: %v", err)
	}

	getTemplate := func() *x509.CertificateRequest {
		// use dynamic ips
		if getIPs != nil {
			tmpIPs, err := getIPs()
			if err == nil && len(tmpIPs) != 0 {
				klog.V(4).Infof("the latest tunnel server's ips=%#+v", tmpIPs)
				ips = tmpIPs
			}
		}
		return &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   commonName,
				Organization: organizations,
			},
			DNSNames:    dnsNames,
			IPAddresses: ips,
		}
	}

	certManager, err := certificate.NewManager(&certificate.Config{
		ClientsetFn: func(current *tls.Certificate) (kubernetes.Interface, error) {
			return clientset, nil
		},
		SignerName:       signerName,
		GetTemplate:      getTemplate,
		Usages:           keyUsages,
		CertificateStore: certificateStore,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server certificate manager: %v", err)
	}

	return certManager, nil
}
