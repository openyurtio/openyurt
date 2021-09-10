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

package tracerequest

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestGetNodeIP(t *testing.T) {
	tests := []struct {
		desc   string
		node   corev1.Node
		expect string
	}{
		{
			desc: "get internal ip for node",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Address: "10.10.102.61",
							Type:    corev1.NodeInternalIP,
						},
					},
				},
			},
			expect: "10.10.102.61",
		},

		{
			desc: "get other ip for node",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Address: "192.168.1.2",
							Type:    corev1.NodeExternalIP,
						},
					},
				},
			},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			nodeIP := getNodeIP(&tt.node)
			if nodeIP != tt.expect {
				t.Errorf("get ip for node failed")
			}
		})
	}
}
