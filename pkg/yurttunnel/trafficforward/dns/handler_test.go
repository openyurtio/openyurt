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
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

func TestAddNode(t *testing.T) {
	clusterIP := "1.2.3.4"
	testcases := map[string]struct {
		node                  *corev1.Node
		defaultTunnelServerIP string
		kubeClient            *fake.Clientset
		records               []string
	}{
		"add a edge node without default tunnel server ip": {
			node: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: formatDNSRecord(clusterIP, "node2"),
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node2"),
				formatDNSRecord(clusterIP, "node1"),
			},
		},
		"add a cloud node with default tunnel server ip": {
			node: &corev1.Node{
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
			defaultTunnelServerIP: clusterIP,
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: formatDNSRecord(clusterIP, "node2"),
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node2"),
				formatDNSRecord("192.168.1.2", "node1"),
			},
		},
		"add a edge node with empty configmap": {
			node: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: "",
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node1"),
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient: tt.kubeClient,
				queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
			}

			if len(tt.defaultTunnelServerIP) != 0 {
				dnsCtl.tunnelServerIP = tt.defaultTunnelServerIP
			}

			dnsCtl.addNode(tt.node)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()
			cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get configmap, %v", resErr)
			}
			cmRecords := strings.Split(cm.Data[constants.YurttunnelDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.records)
			if !reflect.DeepEqual(cmRecords, tt.records) {
				t.Errorf("expect to get records %v, but got %v", tt.records, cmRecords)
			}
		})
	}
}

func TestUpdateNode(t *testing.T) {
	clusterIP := "1.2.3.4"
	testcases := map[string]struct {
		oldNode    *corev1.Node
		newNode    *corev1.Node
		kubeClient *fake.Clientset
		records    []string
	}{
		"update a edge node to cloud node": {
			oldNode: &corev1.Node{
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
			newNode: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: strings.Join([]string{
							formatDNSRecord(clusterIP, "node1"),
							formatDNSRecord(clusterIP, "node2")},
							"\n"),
					},
				},
			),
			records: []string{
				formatDNSRecord("192.168.1.2", "node1"),
				formatDNSRecord(clusterIP, "node2"),
			},
		},
		"update a cloud node to edge node": {
			oldNode: &corev1.Node{
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
			newNode: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: strings.Join([]string{
							formatDNSRecord("192.168.1.2", "node1"),
							formatDNSRecord(clusterIP, "node2")},
							"\n"),
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node1"),
				formatDNSRecord(clusterIP, "node2"),
			},
		},
		"update node ip of a edge node": {
			oldNode: &corev1.Node{
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
			newNode: &corev1.Node{
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
							Address: "192.168.1.3",
						},
					},
				},
			},
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: strings.Join([]string{
							formatDNSRecord(clusterIP, "node1"),
							formatDNSRecord(clusterIP, "node2")},
							"\n"),
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node1"),
				formatDNSRecord(clusterIP, "node2"),
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient: tt.kubeClient,
				queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
			}

			dnsCtl.updateNode(tt.oldNode, tt.newNode)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()
			cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get configmap, %v", resErr)
			}
			cmRecords := strings.Split(cm.Data[constants.YurttunnelDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.records)
			if !reflect.DeepEqual(cmRecords, tt.records) {
				t.Errorf("expect to get records %v, but got %v", tt.records, cmRecords)
			}
		})
	}
}

func TestDeleteNode(t *testing.T) {
	clusterIP := "1.2.3.4"
	testcases := map[string]struct {
		node       *corev1.Node
		kubeClient *fake.Clientset
		records    []string
	}{
		"delete a node and one node left": {
			node: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: strings.Join([]string{
							formatDNSRecord(clusterIP, "node1"),
							formatDNSRecord(clusterIP, "node2")},
							"\n"),
					},
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node2"),
			},
		},
		"delete a node and no node left": {
			node: &corev1.Node{
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
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      yurttunnelDNSRecordConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						constants.YurttunnelDNSRecordNodeDataKey: formatDNSRecord("192.168.1.2", "node1"),
					},
				},
			),
			records: []string{},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient: tt.kubeClient,
				queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
			}

			dnsCtl.deleteNode(tt.node)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()
			cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get configmap, %v", resErr)
			}

			if len(cm.Data[constants.YurttunnelDNSRecordNodeDataKey]) == 0 && len(tt.records) == 0 {
				t.Logf("get empty records, and expect records %v", tt.records)
			} else {
				cmRecords := strings.Split(cm.Data[constants.YurttunnelDNSRecordNodeDataKey], "\n")
				sort.Strings(tt.records)
				if !reflect.DeepEqual(cmRecords, tt.records) {
					t.Errorf("expect to get records %v, but got %v", tt.records, cmRecords)
				}
			}
		})
	}
}

func TestAddConfigMap(t *testing.T) {
	testcases := map[string]struct {
		cm         *corev1.ConfigMap
		kubeClient *fake.Clientset
		ports      []corev1.ServicePort
	}{
		"add a configmap": {
			cm: &corev1.ConfigMap{
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
			kubeClient: fake.NewSimpleClientset(
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
								TargetPort: intstr.FromInt(10263),
							},
						},
					},
				},
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
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10263),
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
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient:         tt.kubeClient,
				queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
				listenInsecureAddr: "127.0.0.1:10264",
				listenSecureAddr:   "127.0.0.1:10263",
			}

			dnsCtl.addConfigMap(tt.cm)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()

			svc, resErr := tt.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).Get(context.TODO(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get service, %v", resErr)
			}

			if !isTheSamePorts(tt.ports, svc.Spec.Ports) {
				t.Errorf("expect to get ports %v, but got %v", tt.ports, svc.Spec.Ports)
			}
		})
	}
}

func TestUpdateConfigMap(t *testing.T) {
	testcases := map[string]struct {
		oldCm      *corev1.ConfigMap
		newCm      *corev1.ConfigMap
		kubeClient *fake.Clientset
		ports      []corev1.ServicePort
	}{
		"update configmap normally": {
			oldCm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.YurttunnelServerDnatConfigMapName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Data: map[string]string{
					"http-proxy-ports":  "1235, 1236",
					"https-proxy-ports": "1237, 1238",
				},
			},
			newCm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.YurttunnelServerDnatConfigMapName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Data: map[string]string{
					"http-proxy-ports":  "1237, 1238",
					"https-proxy-ports": "1235, 1236",
				},
			},
			kubeClient: fake.NewSimpleClientset(
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
								TargetPort: intstr.FromInt(10263),
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
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.YurttunnelServerDnatConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						"http-proxy-ports":  "1237, 1238",
						"https-proxy-ports": "1235, 1236",
					},
				},
			),
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1235",
					Protocol:   corev1.ProtocolTCP,
					Port:       1235,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1236",
					Protocol:   corev1.ProtocolTCP,
					Port:       1236,
					TargetPort: intstr.FromInt(10263),
				},
				{
					Name:       "dnat-1237",
					Protocol:   corev1.ProtocolTCP,
					Port:       1237,
					TargetPort: intstr.FromInt(10264),
				},
				{
					Name:       "dnat-1238",
					Protocol:   corev1.ProtocolTCP,
					Port:       1238,
					TargetPort: intstr.FromInt(10264),
				},
			},
		},
		"configmap is not updated": {
			oldCm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.YurttunnelServerDnatConfigMapName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Data: map[string]string{
					"http-proxy-ports":  "1235, 1236",
					"https-proxy-ports": "1237, 1238",
				},
			},
			newCm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.YurttunnelServerDnatConfigMapName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Data: map[string]string{
					"http-proxy-ports":  "1235, 1236",
					"https-proxy-ports": "1237, 1238",
				},
			},
			kubeClient: fake.NewSimpleClientset(
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
								TargetPort: intstr.FromInt(10263),
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
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.YurttunnelServerDnatConfigMapName,
						Namespace: constants.YurttunnelServerServiceNs,
					},
					Data: map[string]string{
						"http-proxy-ports":  "1237, 1238",
						"https-proxy-ports": "1235, 1236",
					},
				},
			),
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10263),
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
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient:         tt.kubeClient,
				queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
				listenInsecureAddr: "127.0.0.1:10264",
				listenSecureAddr:   "127.0.0.1:10263",
			}

			dnsCtl.updateConfigMap(tt.oldCm, tt.newCm)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()

			svc, resErr := tt.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).Get(context.TODO(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get service, %v", resErr)
			}

			if !isTheSamePorts(tt.ports, svc.Spec.Ports) {
				t.Errorf("expect to get ports %v, but got %v", tt.ports, svc.Spec.Ports)
			}
		})
	}
}

func TestDeleteConfigMap(t *testing.T) {
	testcases := map[string]struct {
		cm         *corev1.ConfigMap
		kubeClient *fake.Clientset
		ports      []corev1.ServicePort
	}{
		"delete configmap normally": {
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.YurttunnelServerDnatConfigMapName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Data: map[string]string{
					"http-proxy-ports":  "1235, 1236",
					"https-proxy-ports": "1237, 1238",
				},
			},
			kubeClient: fake.NewSimpleClientset(
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
								TargetPort: intstr.FromInt(10263),
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
				},
			),
			ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolUDP,
					Port:       1233,
					TargetPort: intstr.FromInt(10263),
				},
			},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnsCtl := &coreDNSRecordController{
				kubeClient:         tt.kubeClient,
				queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
				listenInsecureAddr: "127.0.0.1:10264",
				listenSecureAddr:   "127.0.0.1:10263",
			}

			dnsCtl.deleteConfigMap(tt.cm)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()

			svc, resErr := tt.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).Get(context.TODO(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get service, %v", resErr)
			}

			if !isTheSamePorts(tt.ports, svc.Spec.Ports) {
				t.Errorf("expect to get ports %v, but got %v", tt.ports, svc.Spec.Ports)
			}
		})
	}
}

func TestAddService(t *testing.T) {
	clusterIP := "1.2.3.4"
	testcases := map[string]struct {
		svc                   *corev1.Service
		defaultTunnelServerIP string
		kubeClient            *fake.Clientset
		records               []string
	}{
		"add service normally": {
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.YurttunnelServerInternalServiceName,
					Namespace: constants.YurttunnelServerServiceNs,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: clusterIP,
				},
			},
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
							"openyurt.io/is-edge-worker": "false",
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
				},
			),
			records: []string{
				formatDNSRecord(clusterIP, "node1"),
				formatDNSRecord("192.168.1.3", "node2"),
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
				kubeClient: tt.kubeClient,
				nodeLister: nodeLister,
				queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
			}

			dnsCtl.addService(tt.svc)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					if dnsCtl.queue.Len() == 0 {
						dnsCtl.queue.ShutDown()
					}
				}
			}()
			dnsCtl.worker()
			cm, resErr := tt.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Get(context.TODO(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
			if resErr != nil {
				t.Errorf("failed to get configmap, %v", resErr)
			}
			cmRecords := strings.Split(cm.Data[constants.YurttunnelDNSRecordNodeDataKey], "\n")

			sort.Strings(tt.records)
			if !reflect.DeepEqual(cmRecords, tt.records) {
				t.Errorf("expect to get records %v, but got %v", tt.records, cmRecords)
			}
		})
	}
}
