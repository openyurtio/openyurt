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
	"encoding/pem"
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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	clicert "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/client-go/tools/cache"
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
		dnsNames, ips, err = getYurttunelServerDNSandIP(clientset)
		if err == nil {
			return true, nil
		}
		klog.Errorf("failed to get DNS names and ips: %s", err)
		return false, nil
	}, stopCh)
	// add user specified DNS anems and IP addresses
	dnsNames = append(dnsNames, strings.Split(clCertNames, ",")...)
	for _, ipstr := range strings.Split(clIPs, ",") {
		ips = append(ips, net.ParseIP(ipstr))
	}

	return newCertManager(
		clientset,
		"yurttunnel-server",
		constants.YurttunnelServerCertDir,
		constants.YurttunneServerCSRCN,
		[]string{constants.YurttunneServerCSROrg, constants.YurttunnelCSROrg},
		dnsNames, ips)
}

// getYurttunelServerDNSandIP gets DNS names and IPS that will be added into
// the yurttunnel-server certificate
func getYurttunelServerDNSandIP(
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
	for i, addr := range nodeLst.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			ips = append(ips, net.ParseIP(addr.Address))
		}
		// there is no qualified address (i.e. NodeInternalIP)
		if i == len(nodeLst.Items[0].Status.Addresses)-1 {
			return dnsNames, ips, errors.New("can't find node IP")
		}
	}
	return dnsNames, ips, nil
}

// NewYurttunnelAgentCertManager creates a certificate manager for
// the yurttunel-agent
func NewYurttunnelAgentCertManager(
	clientset kubernetes.Interface) (certificate.Manager, error) {
	nodeIP, err := getAgentNodeIP(clientset, os.Getenv("NODE_NAME"))
	if err != nil {
		return nil, err
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

// getAgentNodeIP returns the IP of the node whose name is nodeName
func getAgentNodeIP(
	clientset kubernetes.Interface,
	nodeName string) (string, error) {
	node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("fail to get node(%s): %s", nodeName, err)
		return "", err
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	klog.Errorf("node(%s) doesn't have internalIP", nodeName)
	return "", fmt.Errorf("internalIP of node(%s) is not found", nodeName)
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
				CommonName:   constants.YurttunneServerCSRCN,
				Organization: organizations,
			},
			DNSNames:    dnsNames,
			IPAddresses: ipAddrs,
		}
	}

	serverCertManager, err := certificate.NewManager(&certificate.Config{
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

	return serverCertManager, nil
}

// ApprApproveYurttunnelCSR starts a csr controller that automatically approves
// the yurttunel related csr
func ApproveYurttunnelCSR(
	clientset kubernetes.Interface,
	stopCh <-chan struct{}) {
	informerFactory := informers.
		NewSharedInformerFactory(clientset, 10*time.Minute)
	csrInformer := informerFactory.Certificates().V1beta1().
		CertificateSigningRequests().Informer()
	csrInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			approveYurttunnelCSR(obj, clientset)
		},
		UpdateFunc: func(old, new interface{}) {
			approveYurttunnelCSR(new, clientset)
		},
	})
	go csrInformer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, csrInformer.HasSynced) {
		klog.Error("sync csr timeout")
	}
	<-stopCh
}

// approveYurttunnelCSR checks the csr status, if it is neither approved nor
// denied, it will try to approve the csr.
func approveYurttunnelCSR(obj interface{}, clientset kubernetes.Interface) {
	csr := obj.(*certificates.CertificateSigningRequest)
	if !isYurttunelCSR(csr) {
		klog.Infof("csr(%s) is not Yurttunnel csr", csr.GetName())
		return
	}

	approved, denied := checkCertApprovalCondition(&csr.Status)
	if approved {
		klog.Infof("csr(%s) is approved", csr.GetName())
		return
	}

	if denied {
		klog.Infof("csr(%s) is denied", csr.GetName())
		return
	}

	// approve the edge tunnel server csr
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificates.CertificateSigningRequestCondition{
			Type:    certificates.CertificateApproved,
			Reason:  "AutoApproved",
			Message: "Auto approving edge tunnel server csr by edge tunnel server itself.",
		})

	csrClient := clientset.CertificatesV1beta1().CertificateSigningRequests()
	result, err := csrClient.UpdateApproval(csr)
	if err != nil {
		if result == nil {
			klog.Errorf("failed to approve yurttunnel csr, %v", err)
		} else {
			klog.Errorf("failed to approve yurttunnel csr(%s), %v", result.Name, err)
		}
	}
	klog.Infof("successfully approve yurttunnel csr(%s)", result.Name)
}

// isYurttunelCSR checks if given csr is a yurtunnel related csr, i.e.,
// the organizations' list contains "openyurt:yurttunnel"
func isYurttunelCSR(csr *certificates.CertificateSigningRequest) bool {
	pemBytes := csr.Spec.Request
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false
	}
	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false
	}
	for i, org := range x509cr.Subject.Organization {
		if org == constants.YurttunnelCSROrg {
			break
		}
		if i == len(x509cr.Subject.Organization)-1 {
			return false
		}
	}
	return true
}

// checkCertApprovalCondition checks if the given csr's status is
// approved or denied
func checkCertApprovalCondition(
	status *certificates.CertificateSigningRequestStatus) (
	approved bool, denied bool) {
	for _, c := range status.Conditions {
		if c.Type == certificates.CertificateApproved {
			approved = true
		}
		if c.Type == certificates.CertificateDenied {
			denied = true
		}
	}
	return
}
