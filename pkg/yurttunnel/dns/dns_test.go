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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
