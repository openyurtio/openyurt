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
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	anpagent "sigs.k8s.io/apiserver-network-proxy/pkg/agent"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
	"github.com/alibaba/openyurt/pkg/yurttunnel/pki/certmanager"
)

// GetServerAddr gets the service address that exposes the yurttunnel-server
func GetTunnelServerAddr(clientset kubernetes.Interface) (string, error) {
	svc, err := clientset.CoreV1().Services(constants.YurttunnelServerServiceNs).
		Get(constants.YurttunnelServerServiceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	_, ips, err := certmanager.GetYurttunelServerDNSandIP(clientset)
	if err != nil {
		return "", err
	}

	if len(ips) <= 1 {
		return "", errors.New("there is no available ip")
	}

	var tcpPort int32
	for _, port := range svc.Spec.Ports {
		if port.Name == constants.YurttunnelServerAgentPortName {
			if svc.Spec.Type == corev1.ServiceTypeNodePort {
				tcpPort = port.NodePort
			} else {
				tcpPort = port.Port
			}
			break
		}
	}

	if tcpPort == 0 {
		return "", errors.New("fail to get the port number")
	}

	var ip net.IP
	for _, tmpIP := range ips {
		// we use the first non-loopback IP address.
		if tmpIP.String() != "127.0.0.1" {
			ip = tmpIP
		}
	}

	return fmt.Sprintf("%s:%d", ip.String(), tcpPort), nil
}

// RunAgent runs the yurttunnel-agent which will try to connect yurttunnel-server
func RunAgent(
	tlsCfg *tls.Config,
	tunnelServerAddr,
	nodeName string,
	stopChan <-chan struct{}) {
	dialOption := grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
	cc := &anpagent.ClientSetConfig{
		Address:                 tunnelServerAddr,
		AgentID:                 nodeName,
		SyncInterval:            5 * time.Second,
		ProbeInterval:           5 * time.Second,
		ReconnectInterval:       5 * time.Second,
		DialOption:              dialOption,
		ServiceAccountTokenPath: "",
	}

	cs := cc.NewAgentClientSet(stopChan)
	cs.Serve()
	klog.Infof("start serving grpc request redirected from yurttunel-server: %s",
		tunnelServerAddr)
}
