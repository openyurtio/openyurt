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

package serveraddr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	utilip "github.com/openyurtio/openyurt/pkg/util/ip"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

type GetIPs func() ([]net.IP, error)

// GetServerAddr gets the service address that exposes the tunnel server for
// tunnel agent to connect
func GetTunnelServerAddr(clientset kubernetes.Interface) (string, error) {
	var (
		ip      net.IP
		host    string
		tcpPort int32
	)

	// get tunnel server resources
	svc, eps, nodeLst, err := getTunnelServerResources(clientset)
	if err != nil {
		return "", err
	}

	dnsNames, ips, err := extractTunnelServerDNSandIPs(svc, []*corev1.Endpoints{eps}, NodeListToNodes(nodeLst))
	if err != nil {
		return "", err
	}

	for _, tmpIP := range ips {
		// we use the first non-loopback IP address.
		if s := tmpIP.String(); s != utilip.DefaultLoopbackIP4 && s != utilip.DefaultLoopbackIP6 {
			ip = tmpIP
			break
		}
	}

	if ip == nil {
		if len(dnsNames) == 0 {
			return "", errors.New("there is no available ip")
		}
		host = dnsNames[0]
	} else {
		host = ip.String()
	}

	for _, port := range svc.Spec.Ports {
		if port.Name == constants.YurttunnelServerAgentPortName {
			tcpPort = port.Port
			if svc.Spec.Type == corev1.ServiceTypeNodePort {
				tcpPort = port.NodePort
			}
			break
		}
	}

	if tcpPort == 0 {
		return "", errors.New("fail to get the port number")
	}

	return net.JoinHostPort(host, strconv.Itoa(int(tcpPort))), nil
}

// GetYurttunelServerDNSandIP gets DNS names and IPS for generating tunnel server certificate.
// the following items are usage:
//   1. dns names and ips will be added into the yurttunnel-server certificate.
//   2. ips may be used by tunnel agent to connect tunnel server
// attention:
//   1. when the type of x-tunnel-server-svc service is LB, make sure the first return ip is LB ip address
//   2. when the type of x-tunnel-server-svc service is ClusterIP, if the x-tunnel-server-external-addr
//      annotation is set, make sure the return ip is annotation setting.
func GetYurttunelServerDNSandIP(
	clientset kubernetes.Interface) ([]string, []net.IP, error) {
	// get tunnel server resources
	svc, eps, nodeLst, err := getTunnelServerResources(clientset)
	if err != nil {
		return []string{}, []net.IP{}, err
	}

	return extractTunnelServerDNSandIPs(svc, []*corev1.Endpoints{eps}, NodeListToNodes(nodeLst))
}

// YurttunelServerAddrManager list the latest tunnel server resources, extract ips and dnsNames from them
func YurttunnelServerAddrManager(factory informers.SharedInformerFactory) ([]string, []net.IP, error) {
	var (
		ips              = make([]net.IP, 0)
		dnsNames         = make([]string, 0)
		internalIp       net.IP
		internalDnsNames = make([]string, 0)
		err              error
	)

	// list yurt-tunnel-server services
	services, err := factory.Core().V1().Services().Lister().List(labels.Everything())
	if err != nil {
		return dnsNames, ips, err
	}

	// list x-tunnel-server-svc endpoints
	eps, err := factory.Core().V1().Endpoints().Lister().List(labels.Everything())
	if err != nil {
		return dnsNames, ips, err
	}

	// list all of cloud nodes
	label := fmt.Sprintf("%s=false", projectinfo.GetEdgeWorkerLabelKey())
	selector, _ := labels.Parse(label)
	// yurttunnel-server will be deployed on one of the cloud nodes
	nodes, err := factory.Core().V1().Nodes().Lister().List(selector)
	if err != nil {
		return dnsNames, ips, err
	}

	// extract ip from the services
	for _, svc := range services {
		if svc.Name == constants.YurttunnelServerServiceName {
			dnsNames, ips, err = extractTunnelServerDNSandIPs(svc, eps, nodes)
		} else {
			// get clusterIP for x-tunnel-server-internal-svc
			if svc.Name == constants.YurttunnelServerInternalServiceName && svc.Spec.ClusterIP != "" && net.ParseIP(svc.Spec.ClusterIP) != nil {
				internalIp = net.ParseIP(svc.Spec.ClusterIP)
				internalDnsNames = GetDefaultDomainsForSvc(svc.Namespace, svc.Name)
			}
		}
	}

	if internalIp != nil {
		ips = append(ips, internalIp)
	}

	if len(internalDnsNames) != 0 {
		dnsNames = append(dnsNames, internalDnsNames...)
	}

	return dnsNames, ips, err
}

// getTunnelServerResources get service, endpoints, and cloud nodes of tunnel server
func getTunnelServerResources(clientset kubernetes.Interface) (*corev1.Service, *corev1.Endpoints, *corev1.NodeList, error) {
	var (
		svc     *corev1.Service
		eps     *corev1.Endpoints
		nodeLst *corev1.NodeList
		err     error
	)
	// get x-tunnel-server-svc service
	svc, err = clientset.CoreV1().
		Services(constants.YurttunnelServerServiceNs).
		Get(context.Background(), constants.YurttunnelServerServiceName, metav1.GetOptions{})
	if err != nil {
		return svc, eps, nodeLst, err
	}

	// get x-tunnel-server-svc endpoints
	eps, err = clientset.CoreV1().
		Endpoints(constants.YurttunnelEndpointsNs).
		Get(context.Background(), constants.YurttunnelEndpointsName, metav1.GetOptions{})
	if err != nil {
		return svc, eps, nodeLst, err
	}

	// get all of cloud nodes when tunnel server expose by NodePort service
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		labelSelector := fmt.Sprintf("%s=false", projectinfo.GetEdgeWorkerLabelKey())
		// yurttunnel-server will be deployed on one of the cloud nodes
		nodeLst, err = clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return svc, eps, nodeLst, err
		}
	}

	return svc, eps, nodeLst, nil
}

// extractTunnelServerDNSandIPs extract tunnel server dnsNames and ips from service and endpoints
func extractTunnelServerDNSandIPs(svc *corev1.Service, eps []*corev1.Endpoints, nodes []*corev1.Node) ([]string, []net.IP, error) {
	var (
		dnsNames = make([]string, 0)
		ips      = make([]net.IP, 0)
		err      error
	)

	// 1. extract dns and ip from the service
	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		// make sure lb ip address is the first index in return ips slice
		dnsNames, ips, err = getLoadBalancerDNSandIP(svc)
	case corev1.ServiceTypeClusterIP:
		// make sure annotation setting address is the first index in return ips slice
		dnsNames, ips, err = getDNSandIPFromAnnotations(svc)
	case corev1.ServiceTypeNodePort:
		dnsNames, ips, err = getNodePortDNSandIP(nodes)
	default:
		err = fmt.Errorf("unsupported service type: %s", string(svc.Spec.Type))
	}

	if err != nil {
		return dnsNames, ips, err
	}

	// 2. extract dns and ip from ClusterIP info
	dnsNames = append(dnsNames, GetDefaultDomainsForSvc(svc.Namespace, svc.Name)...)
	if svc.Spec.ClusterIP != "None" {
		ips = append(ips, net.ParseIP(svc.Spec.ClusterIP))
	}
	ips = append(ips, net.ParseIP(utilip.DefaultLoopbackIP4), net.ParseIP(utilip.DefaultLoopbackIP6))

	// 3. extract dns and ip from the endpoint
	for _, eps := range eps {
		if eps.Name == constants.YurttunnelEndpointsName {
			for _, ss := range eps.Subsets {
				for _, addr := range ss.Addresses {
					if len(addr.IP) != 0 {
						ips = append(ips, net.ParseIP(addr.IP))
					}

					if len(addr.Hostname) != 0 {
						dnsNames = append(dnsNames, addr.Hostname)
					}
				}
			}
		}
	}

	return dnsNames, ips, nil
}

// getLoadBalancerDNSandIP gets the DNS names and IPs from the LoadBalancer service.
func getLoadBalancerDNSandIP(svc *corev1.Service) ([]string, []net.IP, error) {
	var (
		dnsNames = make([]string, 0)
		ips      = make([]net.IP, 0)
	)

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

// getDNSandIPFromAnnotations gets the DNS names and IPs from the annotations for ClusterIP service.
func getDNSandIPFromAnnotations(svc *corev1.Service) ([]string, []net.IP, error) {
	var (
		dnsNames = make([]string, 0)
		ips      = make([]net.IP, 0)
	)

	if addr, ok := svc.Annotations[constants.YurttunnelServerExternalAddrKey]; ok {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return dnsNames, ips, err
		}

		ip := net.ParseIP(host)
		if ip != nil {
			ips = append(ips, ip)
		} else {
			klog.Warningf("annotation %s(%s) of %s service is not ip",
				constants.YurttunnelServerExternalAddrKey, host, constants.YurttunnelServerServiceName)
			dnsNames = append(dnsNames, host)
		}
	}

	return dnsNames, ips, nil
}

// getClusterIPDNSandIP gets the DNS names and IPs from the NodePort service
func getNodePortDNSandIP(nodes []*corev1.Node) ([]string, []net.IP, error) {
	var (
		dnsNames = make([]string, 0)
		ips      = make([]net.IP, 0)
		ipFound  bool
	)

	if len(nodes) == 0 {
		return dnsNames, ips, errors.New("there is no cloud node")
	}

	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				ipFound = true
				ips = append(ips, net.ParseIP(addr.Address))
			}
			if addr.Type == corev1.NodeHostName {
				dnsNames = append(dnsNames, addr.Address)
			}
		}
	}

	if !ipFound {
		// there is no qualified address (i.e. NodeInternalIP)
		return dnsNames, ips, errors.New("can't find node IP")
	}

	return dnsNames, ips, nil
}

// getDefaultDomainsForSvc get default domains for specified service
func GetDefaultDomainsForSvc(ns, name string) []string {
	domains := make([]string, 0)
	if len(ns) == 0 || len(name) == 0 {
		return domains
	}

	domains = append(domains, name)
	domains = append(domains, fmt.Sprintf("%s.%s", name, ns))
	domains = append(domains, fmt.Sprintf("%s.%s.svc", name, ns))
	domains = append(domains, fmt.Sprintf("%s.%s.svc.cluster.local", name, ns))

	return domains
}

func NodeListToNodes(nodeLst *corev1.NodeList) []*corev1.Node {
	nodes := make([]*corev1.Node, 0)
	if nodeLst != nil {
		for _, node := range nodeLst.Items {
			nodes = append(nodes, &node)
		}
	}
	return nodes
}
