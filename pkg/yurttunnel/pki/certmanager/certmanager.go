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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	certificates "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clicert "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/client-go/util/certificate"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
)

// NewYurttunnelServerCertManager creates a certificate manager for
// the yurttunnel-server
func NewYurttunnelServerCertManager(
	clientset kubernetes.Interface,
	clCertNames,
	clIPs string,
	stopCh <-chan struct{}) (certificate.Manager, error) {
	// get server DNS names and IPs
	var (
		dnsNames = []string{}
		ips      = []net.IP{}
		err      error
	)
	_ = wait.PollUntil(5*time.Second, func() (bool, error) {
		dnsNames, ips, err = GetYurttunelServerDNSandIP(clientset)
		if err == nil {
			return true, nil
		}
		klog.Errorf("failed to get DNS names and ips: %s", err)
		return false, nil
	}, stopCh)
	// add user specified DNS anems and IP addresses
	if clCertNames != "" {
		dnsNames = append(dnsNames, strings.Split(clCertNames, ",")...)
	}
	if clIPs != "" {
		for _, ipstr := range strings.Split(clIPs, ",") {
			ips = append(ips, net.ParseIP(ipstr))
		}
	}
	return newCertManager(
		clientset,
		"yurttunnel-server",
		constants.YurttunnelServerCertDir,
		constants.YurttunneServerCSRCN,
		[]string{constants.YurttunneServerCSROrg, constants.YurttunnelCSROrg},
		dnsNames, ips)
}

// GetYurttunelServerDNSandIP gets DNS names and IPS that will be added into
// the yurttunnel-server certificate
func GetYurttunelServerDNSandIP(
	clientset kubernetes.Interface) ([]string, []net.IP, error) {
	var (
		dnsNames = make([]string, 0)
		ips      = make([]net.IP, 0)
		err      error
	)
	s, err := clientset.CoreV1().
		Services(constants.YurttunnelServerServiceNs).
		Get(constants.YurttunnelServerServiceName, metav1.GetOptions{})
	if err != nil {
		return dnsNames, ips, err
	}

	// extract dns and ip from the service info
	dnsNames = append(dnsNames, fmt.Sprintf("%s.%s", s.Name, s.Namespace))
	if s.Spec.ClusterIP != "None" {
		ips = append(ips, net.ParseIP(s.Spec.ClusterIP))
	}
	ips = append(ips, net.ParseIP("127.0.0.1"))

	switch s.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		return getLoadBalancerDNSandIP(s, dnsNames, ips)
	case corev1.ServiceTypeClusterIP:
		return getClusterIPDNSandIP(s, dnsNames, ips)
	case corev1.ServiceTypeNodePort:
		return getNodePortDNSandIP(clientset, dnsNames, ips)
	default:
		return dnsNames, ips,
			fmt.Errorf("unsupported service type: %s", string(s.Spec.Type))
	}
}

// getLoadBalancerDNSandIP gets the DNS names and IPs from the
// LoadBalancer service
func getLoadBalancerDNSandIP(
	svc *corev1.Service,
	dnsNames []string,
	ips []net.IP) ([]string, []net.IP, error) {
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return dnsNames, ips, errors.New("load balancer is not ready")
	}
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			ips = append(ips, net.ParseIP(ingress.IP))
		}

		if ingress.Hostname != "" {
			dnsNames = append(dnsNames, ingress.Hostname)
		}
	}
	return dnsNames, ips, nil
}

// getClusterIPDNSandIP gets the DNS names and IPs from the ClusterIP service
func getClusterIPDNSandIP(
	svc *corev1.Service,
	dnsNames []string,
	ips []net.IP) ([]string, []net.IP, error) {
	if addr, ok := svc.Annotations[constants.YurttunnelServerExternalAddrKey]; ok {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return dnsNames, ips, err
		}
		ips = append(ips, net.ParseIP(host))
	}
	return dnsNames, ips, nil
}

// getClusterIPDNSandIP gets the DNS names and IPs from the NodePort service
func getNodePortDNSandIP(
	clientset kubernetes.Interface,
	dnsNames []string,
	ips []net.IP) ([]string, []net.IP, error) {
	labelSelector := fmt.Sprintf("%s=false", constants.YurtEdgeNodeLabel)
	// yurttunnel-server will be deployed on one of the cloud nodes
	nodeLst, err := clientset.CoreV1().Nodes().List(
		metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return dnsNames, ips, err
	}
	if len(nodeLst.Items) == 0 {
		return dnsNames, ips, errors.New("there is no cloud node")
	}
	var ipFound bool
	for _, addr := range nodeLst.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			ipFound = true
			ips = append(ips, net.ParseIP(addr.Address))
		}
	}
	if !ipFound {
		// there is no qualified address (i.e. NodeInternalIP)
		return dnsNames, ips, errors.New("can't find node IP")
	}
	return dnsNames, ips, nil
}

// NewYurttunnelAgentCertManager creates a certificate manager for
// the yurttunel-agent
func NewYurttunnelAgentCertManager(
	clientset kubernetes.Interface) (certificate.Manager, error) {
	// As yurttunnel-agent will run on the edge node with Host network mode,
	// we can use the status.podIP as the node IP
	nodeIP := os.Getenv(constants.YurttunnelAgentPodIPEnv)
	if nodeIP == "" {
		return nil, fmt.Errorf("env %s is not set",
			constants.YurttunnelAgentPodIPEnv)
	}

	return newCertManager(
		clientset,
		"yurttunnel-agent",
		constants.YurttunnelAgentCertDir,
		constants.YurttunnelAgentCSRCN,
		[]string{constants.YurttunnelCSROrg},
		[]string{os.Getenv("NODE_NAME")},
		[]net.IP{net.ParseIP(nodeIP)})
}

// NewCertManager creates a certificate manager that will generates a
// certificate by sending a csr to the apiserver
func newCertManager(
	clientset kubernetes.Interface,
	componentName,
	certDir,
	commonName string,
	organizations,
	dnsNames []string, ipAddrs []net.IP) (certificate.Manager, error) {
	certificateStore, err :=
		certificate.NewFileStore(componentName, certDir, certDir, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the server certificate store: %v", err)
	}

	getTemplate := func() *x509.CertificateRequest {
		return &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   commonName,
				Organization: organizations,
			},
			DNSNames:    dnsNames,
			IPAddresses: ipAddrs,
		}
	}

	certManager, err := certificate.NewManager(&certificate.Config{
		ClientFn: func(current *tls.Certificate) (clicert.CertificateSigningRequestInterface, error) {
			return clientset.CertificatesV1beta1().CertificateSigningRequests(), nil
		},
		GetTemplate: getTemplate,
		Usages: []certificates.KeyUsage{
			certificates.UsageAny,
		},
		CertificateStore: certificateStore,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server certificate manager: %v", err)
	}

	return certManager, nil
}
