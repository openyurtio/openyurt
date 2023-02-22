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
	"net"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

func TestGetDefaultDomainsForSvc(t *testing.T) {
	tests := []struct {
		desc        string
		ns          string
		name        string
		expectValue []string
	}{
		{
			desc:        "empty ns",
			ns:          "",
			name:        "test-svc",
			expectValue: []string{},
		},
		{
			desc:        "empty name",
			ns:          "default",
			name:        "",
			expectValue: []string{},
		},
		{
			desc: "get default domains for test-svc in default namespace",
			ns:   "default",
			name: "test-svc",
			expectValue: []string{
				"test-svc",
				"test-svc.default",
				"test-svc.default.svc",
				"test-svc.default.svc.cluster.local",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			actValue := GetDefaultDomainsForSvc(tt.ns, tt.name)
			if !reflect.DeepEqual(actValue, tt.expectValue) {
				t.Errorf("the value we want is %+v, but the actual is %+v", tt.expectValue, actValue)
			}
		})
	}
}

func TestGetNodePortDNSandIP(t *testing.T) {
	type ExpectValue struct {
		dnsNames []string
		ips      []net.IP
	}

	tests := []struct {
		desc        string
		nodes       []*corev1.Node
		expectValue ExpectValue
	}{
		{
			desc:  "there is no cloud node",
			nodes: []*corev1.Node{},
			expectValue: ExpectValue{
				ips:      []net.IP{},
				dnsNames: []string{},
			},
		},
		{
			desc: "many nodes with qualified ips and dns names",
			nodes: []*corev1.Node{
				{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-1",
							},
						},
					},
				},
				{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.2",
							},
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-2",
							},
						},
					},
				},
				{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.3",
							},
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-3",
							},
						},
					},
				},
			},

			expectValue: ExpectValue{
				ips: []net.IP{
					net.ParseIP("192.168.1.1"),
					net.ParseIP("192.168.1.2"),
					net.ParseIP("192.168.1.3"),
				},
				dnsNames: []string{
					"cloud-node-1",
					"cloud-node-2",
					"cloud-node-3",
				},
			},
		},
		{
			desc: "Many IPs",
			nodes: []*corev1.Node{
				{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-1",
							},
							{
								Type:    corev1.NodeExternalDNS,
								Address: "openyurt.io",
							},
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.4",
							},
						},
					},
				},
			},

			expectValue: ExpectValue{
				ips: []net.IP{
					net.ParseIP("192.168.1.1"),
					net.ParseIP("192.168.1.4"),
				},
				dnsNames: []string{
					"cloud-node-1",
				},
			},
		},

		{
			desc: "there is no internal ip",
			nodes: []*corev1.Node{
				{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-1",
							},
							{
								Type:    corev1.NodeExternalDNS,
								Address: "openyurt.io",
							},
						},
					},
				},
			},

			expectValue: ExpectValue{
				ips: []net.IP{},
				dnsNames: []string{
					"cloud-node-1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			dnsNames, ips, _ := getNodePortDNSandIP(tt.nodes)
			if !reflect.DeepEqual(ips, tt.expectValue.ips) {
				t.Errorf("the ips we want is %v, but the actual is %v", tt.expectValue.ips, ips)
			}
			if !reflect.DeepEqual(dnsNames, tt.expectValue.dnsNames) {
				t.Errorf("the dns names we want is %v, but the actual is %v", tt.expectValue.dnsNames, dnsNames)
			}
		})
	}

}

func TestGetDNSandIPFromAnnotations(t *testing.T) {
	type ExpectValue struct {
		dnsNames []string
		ips      []net.IP
	}

	tests := []struct {
		desc        string
		svc         corev1.Service
		expectValue ExpectValue
	}{
		{
			desc: "there is constants.YurttunnelServerExternalAddrKey ips which is 192.168.1.2",
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.YurttunnelServerExternalAddrKey: "192.168.1.2:8080",
					},
				},
			},
			expectValue: ExpectValue{
				ips: []net.IP{
					net.ParseIP("192.168.1.2"),
				},
				dnsNames: []string{},
			},
		},
		{
			desc: "there is constants.YurttunnelServerExternalAddrKey dnsnames which is openyurt.io:80",
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.YurttunnelServerExternalAddrKey: "openyurt.io:80",
					},
				},
			},
			expectValue: ExpectValue{
				dnsNames: []string{
					"openyurt.io",
				},
				ips: []net.IP{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			dnsNames, ips, err := getDNSandIPFromAnnotations(&tt.svc)
			if err != nil {
				t.Errorf("error is %v\n", err)
			} else {
				if !reflect.DeepEqual(ips, tt.expectValue.ips) {
					t.Errorf("the ips we want is %v, but the actual is %v", tt.expectValue.ips, ips)
				}
				if !reflect.DeepEqual(dnsNames, tt.expectValue.dnsNames) {
					t.Errorf("the dns names we want is %v, but the actual is %v", tt.expectValue.dnsNames, dnsNames)
				}
			}
		})
	}

}

func TestGetLoadBalancerDNSandIP(t *testing.T) {
	type ExpectValue struct {
		dnsNames []string
		ips      []net.IP
	}

	tests := []struct {
		desc        string
		svc         corev1.Service
		expectValue ExpectValue
	}{
		{
			desc: "load balancer is not ready",
			svc: corev1.Service{
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{},
					},
				},
			},
			expectValue: ExpectValue{
				dnsNames: []string{},
				ips:      []net.IP{},
			},
		},
		{
			desc: "get dns and ips from load balancer",
			svc: corev1.Service{
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "www.xing.com",
								IP:       "192.168.1.2",
							},
							{
								Hostname: "www.test.com",
								IP:       "192.168.1.3",
							},
						},
					},
				},
			},
			expectValue: ExpectValue{
				dnsNames: []string{
					"www.xing.com",
					"www.test.com",
				},
				ips: []net.IP{
					net.ParseIP("192.168.1.2"),
					net.ParseIP("192.168.1.3"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			dnsNames, ips, _ := getLoadBalancerDNSandIP(&tt.svc)

			if !reflect.DeepEqual(ips, tt.expectValue.ips) {
				t.Errorf("the ips we want is %v, but the actual is %v", tt.expectValue.ips, ips)
			}
			if !reflect.DeepEqual(dnsNames, tt.expectValue.dnsNames) {
				t.Errorf("the dns names we want is %v, but the actual is %v", tt.expectValue.dnsNames, dnsNames)
			}

		})
	}
}

func TestExtractTunnelServerDNSandIPs(t *testing.T) {
	type ExpectValue struct {
		dnsNames []string
		ips      []net.IP
	}

	tests := []struct {
		desc        string
		svc         corev1.Service
		eps         []*corev1.Endpoints
		nodes       []*corev1.Node
		expectValue ExpectValue
	}{
		{
			desc: "extract dnsNames and ips for LoadBalancer service",
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeLoadBalancer,
					ClusterIP: "10.10.102.1",
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "load_balancer_svc",
								IP:       "192.168.1.1",
							},
						},
					},
				},
			},
			eps: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelEndpointsName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.2",
									Hostname: "x-tunnel-server-svc-ep-1",
								},
								{
									IP:       "192.168.1.3",
									Hostname: "x-tunnel-server-svc-ep-2",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelServerInternalServiceName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.4",
									Hostname: "x-tunnel-server-internal-svc-ep",
								},
							},
						},
					},
				},
			},
			nodes: []*corev1.Node{},
			expectValue: ExpectValue{
				dnsNames: []string{
					"load_balancer_svc",
					"x-tunnel-server-svc",
					"x-tunnel-server-svc.kube-system",
					"x-tunnel-server-svc.kube-system.svc",
					"x-tunnel-server-svc.kube-system.svc.cluster.local",
					"x-tunnel-server-svc-ep-1",
					"x-tunnel-server-svc-ep-2",
				},
				ips: []net.IP{
					net.ParseIP("192.168.1.1"),
					net.ParseIP("10.10.102.1"),
					net.ParseIP("127.0.0.1"),
					net.ParseIP("::1"),
					net.ParseIP("192.168.1.2"),
					net.ParseIP("192.168.1.3"),
				},
			},
		},
		{
			desc: "extract dnsNames and ips for ClusterIP service",
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
					Annotations: map[string]string{
						constants.YurttunnelServerExternalAddrKey: "cluster_ip_svc:8080",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeClusterIP,
					ClusterIP: "10.10.102.1",
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{},
				},
			},
			eps: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelEndpointsName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.2",
									Hostname: "x-tunnel-server-svc-ep-1",
								},
								{
									IP:       "192.168.1.3",
									Hostname: "x-tunnel-server-svc-ep-2",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelServerInternalServiceName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.4",
									Hostname: "x-tunnel-server-internal-svc-ep",
								},
							},
						},
					},
				},
			},
			nodes: []*corev1.Node{},
			expectValue: ExpectValue{
				dnsNames: []string{
					"cluster_ip_svc",
					"x-tunnel-server-internal-svc",
					"x-tunnel-server-internal-svc.kube-system",
					"x-tunnel-server-internal-svc.kube-system.svc",
					"x-tunnel-server-internal-svc.kube-system.svc.cluster.local",
					"x-tunnel-server-svc-ep-1",
					"x-tunnel-server-svc-ep-2",
				},
				ips: []net.IP{
					net.ParseIP("10.10.102.1"),
					net.ParseIP("127.0.0.1"),
					net.ParseIP("::1"),
					net.ParseIP("192.168.1.2"),
					net.ParseIP("192.168.1.3"),
				},
			},
		},
		{
			desc: "extract dnsNames and ips for NodePort service",
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Type:      corev1.ServiceTypeNodePort,
					ClusterIP: "10.10.102.1",
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{},
				},
			},
			eps: []*corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelEndpointsName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.2",
									Hostname: "x-tunnel-server-svc-ep-1",
								},
								{
									IP:       "192.168.1.3",
									Hostname: "x-tunnel-server-svc-ep-2",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.YurttunnelServerInternalServiceName,
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{
									IP:       "192.168.1.4",
									Hostname: "x-tunnel-server-internal-svc-ep",
								},
							},
						},
					},
				},
			},
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cloud-node-1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.5",
							},
							{
								Type:    corev1.NodeHostName,
								Address: "cloud-node-1",
							},
						},
					},
				},
			},
			expectValue: ExpectValue{
				dnsNames: []string{
					"cloud-node-1",
					"x-tunnel-server-svc",
					"x-tunnel-server-svc.kube-system",
					"x-tunnel-server-svc.kube-system.svc",
					"x-tunnel-server-svc.kube-system.svc.cluster.local",
					"x-tunnel-server-svc-ep-1",
					"x-tunnel-server-svc-ep-2",
				},
				ips: []net.IP{
					net.ParseIP("192.168.1.5"),
					net.ParseIP("10.10.102.1"),
					net.ParseIP("127.0.0.1"),
					net.ParseIP("::1"),
					net.ParseIP("192.168.1.2"),
					net.ParseIP("192.168.1.3"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			dnsNames, ips, _ := extractTunnelServerDNSandIPs(&tt.svc, tt.eps, tt.nodes)
			if !reflect.DeepEqual(ips, tt.expectValue.ips) {
				t.Errorf("the ips we want is %v, but the actual is %v", tt.expectValue.ips, ips)
			}
			if !reflect.DeepEqual(dnsNames, tt.expectValue.dnsNames) {
				t.Errorf("the dns names we want is %v, but the actual is %v", tt.expectValue.dnsNames, dnsNames)
			}

		})
	}
}
