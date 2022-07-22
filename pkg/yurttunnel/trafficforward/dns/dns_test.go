/*
Copyright 2021 The OpenYurt Authors.

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

package dns

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

func TestEnsureCoreDNSRecordConfigMap(t *testing.T) {
	testcases := map[string]struct {
		kubeClient *fake.Clientset
		retErr     bool
		dnsRecords string
	}{
		"configmap has already exist": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: "test",
					},
				},
			),
			dnsRecords: "test",
		},
		"create configmap when it does not exist": {
			kubeClient: fake.NewSimpleClientset(),
			dnsRecords: "",
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient: tt.kubeClient,
			}

			err := dnsCtl.ensureCoreDNSRecordConfigMap()
			if !tt.retErr {
				if err != nil {
					t.Errorf("expect no error, but got %v", err)
				}

				cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
				if resErr != nil {
					t.Errorf("failed to get configmap, %v", resErr)
				}

				if cm.Data[constants.YurttunnelDNSRecordNodeDataKey] != tt.dnsRecords {
					t.Errorf("expect dns records %s, but got %s", tt.dnsRecords, cm.Data[constants.YurttunnelDNSRecordNodeDataKey])
				}
			} else {
				if err == nil {
					t.Errorf("expect a error, but got %v", err)
				}
			}
		})
	}
}

func TestSyncTunnelServerServiceAsWhole(t *testing.T) {
	testcases := map[string]struct {
		kubeClient *fake.Clientset
		retErr     bool
		ports      []corev1.ServicePort
	}{
		"failed to get configmap": {
			kubeClient: fake.NewSimpleClientset(),
			retErr:     true,
		},
		"failed to get service": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.YurttunnelServerDnatConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						"dnat-ports-pair":   "1234=10264",
						"http-proxy-ports":  "1235, 1236",
						"https-proxy-ports": "1237, 1238",
					},
				},
			),
			retErr: true,
		},
		"sync service port normally": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.YurttunnelServerDnatConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						"dnat-ports-pair":   "1234=10264",
						"http-proxy-ports":  "1235, 1236",
						"https-proxy-ports": "1237, 1238",
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:       "https",
								Protocol:   corev1.ProtocolUDP,
								Port:       1233,
								TargetPort: intstr.FromInt(10262),
							},
						},
					},
				},
			),
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10262),
				},
				{
					Name:       "dnat-1234",
					Protocol:   corev1.ProtocolTCP,
					Port:       1234,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1235",
					Protocol:   corev1.ProtocolTCP,
					Port:       1235,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1236",
					Protocol:   corev1.ProtocolTCP,
					Port:       1236,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1237",
					Protocol:   corev1.ProtocolTCP,
					Port:       1237,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1238",
					Protocol:   corev1.ProtocolTCP,
					Port:       1238,
					TargetPort: intstr.FromInt(10263),
				},
			},
		},
		"sync service port with ports overwrite": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.YurttunnelServerDnatConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						"dnat-ports-pair":   "1234=10264",
						"http-proxy-ports":  "1235, 1236, 1239",
						"https-proxy-ports": "1237, 1238",
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:       "https",
								Protocol:   corev1.ProtocolUDP,
								Port:       1233,
								TargetPort: intstr.FromInt(10262),
							},
							{
								Name:       "dnat-1239",
								Protocol:   corev1.ProtocolTCP,
								Port:       1239,
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
			),
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10262),
				},
				{
					Name:       "dnat-1234",
					Protocol:   corev1.ProtocolTCP,
					Port:       1234,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1235",
					Protocol:   corev1.ProtocolTCP,
					Port:       1235,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1236",
					Protocol:   corev1.ProtocolTCP,
					Port:       1236,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1237",
					Protocol:   corev1.ProtocolTCP,
					Port:       1237,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1238",
					Protocol:   corev1.ProtocolTCP,
					Port:       1238,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1239",
					Protocol:   corev1.ProtocolTCP,
					Port:       1239,
					TargetPort: intstr.FromInt(10264),
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient:         tt.kubeClient,
				listenInsecureAddr: "127.0.0.1:10264",
				listenSecureAddr:   "127.0.0.1:10263",
			}

			err := dnsCtl.syncTunnelServerServiceAsWhole()
			if !tt.retErr {
				if err != nil {
					t.Errorf("expect no error, but got %v", err)
				}

				svc, resErr := tt.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).Get(context.TODO(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
				if resErr != nil {
					t.Errorf("failed to get service, %v", resErr)
				}

				if !isTheSamePorts(tt.ports, svc.Spec.Ports) {
					t.Errorf("expect to get ports %v, but got %v", tt.ports, svc.Spec.Ports)
				}
			} else {
				if err == nil {
					t.Errorf("expect a error, but got %v", err)
				}
			}
		})
	}
}

func isTheSamePorts(p1, p2 []corev1.ServicePort) bool {
	if len(p1) != len(p2) {
		return false
	}

	for i := range p1 {
		found := false
		for j := range p2 {
			if p1[i].Name == p2[j].Name && p1[i].Port == p2[j].Port &&
				p1[i].Protocol == p2[j].Protocol && p1[i].TargetPort.IntValue() == p2[j].TargetPort.IntValue() {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for i := range p2 {
		found := false
		for j := range p1 {
			if p2[i].Name == p1[j].Name && p2[i].Port == p1[j].Port &&
				p2[i].Protocol == p1[j].Protocol && p2[i].TargetPort.IntValue() == p1[j].TargetPort.IntValue() {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func TestSyncDNSRecordAsWhole(t *testing.T) {
	clusterIP := "1.2.3.4"
	testcases := map[string]struct {
		kubeClient *fake.Clientset
		retErr     bool
		records    []string
	}{
		"failed to get svc": {
			kubeClient: fake.NewSimpleClientset(),
			retErr:     true,
		},
		"service has no ClusterIP": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
				},
			),
			retErr: true,
		},
		"failed to get configmap": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: clusterIP,
					},
				},
			),
			retErr: true,
		},
		"have edge nodes only": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: clusterIP,
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.2",
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.3",
							},
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node1"),
				formatDNSRecord(clusterIP, "node2"),
			},
		},
		"have cloud nodes only": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: clusterIP,
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.2",
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeExternalIP,
								Address: "192.168.1.3",
							},
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{},
				},
			),
			records: []string{
				formatDNSRecord("192.168.1.2", "node1"),
				formatDNSRecord("192.168.1.3", "node2"),
			},
		},
		"have cloud nodes and edge nodes": {
			kubeClient: fake.NewSimpleClientset(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.YurttunnelServerInternalServiceName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Spec: corev1.ServiceSpec{
						ClusterIP: clusterIP,
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.2",
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "false",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeExternalIP,
								Address: "192.168.1.3",
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.3",
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							"openyurt.io/is-edge-worker": "true",
						},
					},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeExternalIP,
								Address: "192.168.1.4",
							},
						},
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{},
				},
			),
			records: []string{
				formatDNSRecord("192.168.1.2", "node1"),
				formatDNSRecord("192.168.1.3", "node2"),
				formatDNSRecord(clusterIP, "node3"),
				formatDNSRecord(clusterIP, "node4"),
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {

			factory := informers.NewSharedInformerFactory(tt.kubeClient, 0)
			nodeInformer := factory.Core().V1().Nodes()
			nodeInformer.Informer()
			nodeLister := nodeInformer.Lister()

			stopper := make(chan struct{})
			defer close(stopper)
			factory.Start(stopper)
			factory.WaitForCacheSync(stopper)

			dnsCtl := &coreDNSRecordController{
				kubeClient:     tt.kubeClient,
				nodeLister:     nodeLister,
				tunnelServerIP: clusterIP,
			}
			err := dnsCtl.syncDNSRecordAsWhole()
			if !tt.retErr {
				if err != nil {
					t.Errorf("expect no error, but got %v", err)
				}

				cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
				if resErr != nil {
					t.Errorf("failed to get configmap, %v", resErr)
				}
				cmRecords := strings.Split(cm.Data[constants.YurttunnelDNSRecordNodeDataKey], "\n")

				sort.Strings(tt.records)
				if !reflect.DeepEqual(cmRecords, tt.records) {
					t.Errorf("expect to get records %v, but got %v", tt.records, cmRecords)
				}
			} else {
				if err == nil {
					t.Errorf("expect a error, but got %v", err)
				}
			}
		})
	}
}

func TestResolveServicePorts(t *testing.T) {
	testcases := map[string]struct {
		service             *corev1.Service
		currentPorts        []string
		currentPortMappings map[string]string
		expectResult        struct {
			changed  bool
			svcPorts map[string]int
		}
	}{
		"add a new port": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
					},
				},
			},
			currentPorts:        []string{"9510"},
			currentPortMappings: map[string]string{"9510": "1.1.1.1:10264"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: true,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"dnat-9510:TCP:9510:10264": 1,
				}},
		},
		"add port when udp protocol port exists": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
						{
							Name:       "test-udp",
							Protocol:   "UDP",
							Port:       9510,
							TargetPort: intstr.FromString("10264"),
						},
					},
				},
			},
			currentPorts:        []string{"9510"},
			currentPortMappings: map[string]string{"9510": "1.1.1.1:10264"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: true,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"test-udp:UDP:9510:10264":  1,
					"dnat-9510:TCP:9510:10264": 1,
				}},
		},
		"update port with different target port": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
						{
							Name:       "dnat-9510",
							Protocol:   "TCP",
							Port:       9510,
							TargetPort: intstr.FromString("10264"),
						},
					},
				},
			},
			currentPorts:        []string{"9510"},
			currentPortMappings: map[string]string{"9510": "1.1.1.1:10263"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: true,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"dnat-9510:TCP:9510:10263": 1,
				}},
		},
		"add a new port when beyond default port exists": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
						{
							Name:       "dnat-9510",
							Protocol:   "TCP",
							Port:       9510,
							TargetPort: intstr.FromString("10264"),
						},
					},
				},
			},
			currentPorts:        []string{"9510", "9511"},
			currentPortMappings: map[string]string{"9510": "1.1.1.1:10264", "9511": "1.1.1.1:10263"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: true,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"dnat-9510:TCP:9510:10264": 1,
					"dnat-9511:TCP:9511:10263": 1,
				},
			},
		},
		"add a new port meanwhile delete an old port": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
						{
							Name:       "dnat-9510",
							Protocol:   "TCP",
							Port:       9510,
							TargetPort: intstr.FromString("10264"),
						},
					},
				},
			},
			currentPorts:        []string{"9511"},
			currentPortMappings: map[string]string{"9511": "1.1.1.1:10263"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: true,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"dnat-9511:TCP:9511:10263": 1,
				},
			},
		},
		"service ports have not changed": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "x-tunnel-server-internal-svc",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       10255,
							TargetPort: intstr.FromString("10264"),
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       10250,
							TargetPort: intstr.FromString("10263"),
						},
						{
							Name:       "dnat-9510",
							Protocol:   "TCP",
							Port:       9510,
							TargetPort: intstr.FromString("10264"),
						},
					},
				},
			},
			currentPorts:        []string{"9510"},
			currentPortMappings: map[string]string{"9510": "1.1.1.1:10264"},
			expectResult: struct {
				changed  bool
				svcPorts map[string]int
			}{
				changed: false,
				svcPorts: map[string]int{
					"http:TCP:10255:10264":     1,
					"https:TCP:10250:10263":    1,
					"dnat-9510:TCP:9510:10264": 1,
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			changed, svcPorts := resolveServicePorts(tt.service, tt.currentPorts, tt.currentPortMappings)
			if tt.expectResult.changed != changed {
				t.Errorf("expect changed: %v, but got changed: %v", tt.expectResult.changed, changed)
			}

			portsMap := make(map[string]int)
			for _, svcPort := range svcPorts {
				key := fmt.Sprintf("%s:%s:%d:%s", svcPort.Name, svcPort.Protocol, svcPort.Port, svcPort.TargetPort.String())
				if cnt, ok := portsMap[key]; ok {
					portsMap[key] = cnt + 1
				} else {
					portsMap[key] = 1
				}
			}

			// check the servicePorts
			if len(tt.expectResult.svcPorts) != len(portsMap) {
				t.Errorf("expect %d service ports, but got %d service ports", len(tt.expectResult.svcPorts), len(portsMap))
			}

			for k, v := range tt.expectResult.svcPorts {
				if gotV, ok := portsMap[k]; !ok {
					t.Errorf("expect key %s, but not got", k)
				} else if v != gotV {
					t.Errorf("key(%s): expect value %d, but got value %d", k, v, gotV)
				}
			}
		})
	}
}
