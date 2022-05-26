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
	YurtHubCSROrg              = "openyurt:yurthub"
	YurtTunnelCSROrg           = "openyurt:yurttunnel"
	YurtTunnelServerNodeName   = "tunnel-server"
	YurtTunnelProxyClientCSRCN = "tunnel-proxy-client"
	YurtTunnelAgentCSRCN       = "tunnel-agent-client"
)

// NewYurttunnelServerCertManager creates a certificate manager for
// the yurttunnel-server, and the certificate will be used for https server
// that listens for requests from kube-apiserver and other cloud components(like prometheus).
// meanwhile the certificate will also be used for tls server that wait for connections that
// comes from yurt-tunnel-agent.
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
		certificatesv1.KubeletServingSignerName,
		fmt.Sprintf("system:node:%s", YurtTunnelServerNodeName),
		[]string{user.NodesGroup},
		dnsNames,
		[]certificatesv1.KeyUsage{
			certificatesv1.UsageKeyEncipherment,
			certificatesv1.UsageDigitalSignature,
			certificatesv1.UsageServerAuth,
		},
		ips,
		getIPs)
}

// NewTunnelProxyClientCertManager creates a certificate manager for yurttunnel-server.
// and the certificate will be used for handshaking with components(like kubelet) on edge nodes.
// by the way, requests from kube-apiserver or other cloud components(like prometheus) will be forwarded
// to the edge based on the tls connection.
func NewTunnelProxyClientCertManager(clientset kubernetes.Interface, certDir string) (certificate.Manager, error) {
	return newCertManager(
		clientset,
		fmt.Sprintf("%s-proxy-client", projectinfo.GetServerName()),
		certDir,
		certificatesv1.KubeAPIServerClientSignerName,
		YurtTunnelProxyClientCSRCN,
		[]string{YurtTunnelCSROrg},
		[]string{},
		[]certificatesv1.KeyUsage{
			certificatesv1.UsageKeyEncipherment,
			certificatesv1.UsageDigitalSignature,
			certificatesv1.UsageClientAuth,
		},
		[]net.IP{},
		nil)
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
		certificatesv1.KubeAPIServerClientSignerName,
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
		return nil, fmt.Errorf("failed to initialize the server certificate store: %w", err)
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
		return nil, fmt.Errorf("failed to initialize server certificate manager: %w", err)
	}

	return certManager, nil
}
