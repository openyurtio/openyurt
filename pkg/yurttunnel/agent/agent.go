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

package agent

import (
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetServerAddr gets the service address that exposes the yurttunnel-server
func GetTunnelServerAddr(clientset kubernetes.Interface) (string, error) {
	svc, err := clientset.CoreV1().Services(constants.YurttunnelServiceNs).
		Get(constants.YurttunnelServiceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		return getServerAddrLoadBalancer(svc)
	case corev1.ServiceTypeClusterIP:
		return getServerAddrClusterIP(clientset, svc)
	case corev1.ServiceTypeNodePort:
		return getServerAddrNodePort(clientset, svc)
	default:
		return "", fmt.Errorf("unupported service type: %s", svc.Spec.Type)
	}
}

// getServerAddrLoadBalancer gets the service address of the yurttunnel-server
// if the service type is LoadBalancer
func getServerAddrLoadBalancer(
	svc *corev1.Service) (string, error) {
	var tcpPort int32
	for _, port := range svc.Spec.Ports {
		if port.Name == constants.YurttunnelServerAgentPortName {
			tcpPort = port.Port
			break
		}
	}

	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if len(ingress.IP) != 0 {
			return fmt.Sprintf("%s:%d", ingress.IP, tcpPort), nil
		}
	}
	return "", errors.New("can't find qualified ingress")
}

// getServerAddrClusterIP gets the service address of the yurttunnel-server
// if the service type is ClusterIP
func getServerAddrClusterIP(
	clientset kubernetes.Interface,
	svc *corev1.Service) (string, error) {
	if addr, ok := svc.Annotations[constants.YurttunnelServerExternalAddrKey]; ok {
		return addr, nil
	}

	eps, err := clientset.CoreV1().Endpoints(constants.YurttunnelEndpointsNs).
		Get(constants.YurttunnelEndpointsName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, ss := range eps.Subsets {
		if len(ss.Addresses) == 1 && len(ss.Ports) == 1 {
			return fmt.Sprintf("%s:%d", ss.Addresses[0].IP, ss.Ports[0].Port), nil
		}
	}
	return "", errors.New("can't find qualified endpoint subsets")
}

// getServerAddrNodePort gets the service address of the yurttunnel-server
// if the service type is NodePort
func getServerAddrNodePort(
	clientset kubernetes.Interface,
	svc *corev1.Service) (string, error) {
	// get node ip
	labelSelector := "alibabacloud.com/is-edge-worker=false"
	nodeLst, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", err
	}
	if len(nodeLst.Items) == 0 {
		return "", errors.New("there is no cloud node")
	}
	var (
		nodeIP      string
		foundNodeIP bool
	)
	for _, addr := range nodeLst.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			nodeIP = addr.Address
			foundNodeIP = true
		}
	}
	if !foundNodeIP {
		return "", errors.New("can't find node IP")
	}
	// get node port
	var (
		tcpPort      int32
		foundTCPPort bool
	)
	for _, port := range svc.Spec.Ports {
		if port.Name == constants.YurttunnelServerAgentPortName {
			tcpPort = port.NodePort
			foundTCPPort = true
			break
		}
	}
	if !foundTCPPort {
		return "", errors.New("tcp port not found")
	}
	return fmt.Sprintf("%s:%d", nodeIP, tcpPort), nil
}

// RunAgent runs the yurttunnel-agent
func RunAgent(
	tlsCfg *tls.Config,
	serverAddr,
	nodeName string,
	stopChan <-chan struct{}) error {
	return errors.New("NOT IMPLEMENT YET")
}
