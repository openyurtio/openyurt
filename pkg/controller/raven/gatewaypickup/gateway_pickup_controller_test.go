/*
Copyright 2023 The OpenYurt Authors.
Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gatewaypickup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ravenv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/raven/gatewaypickup/config"
)

var (
	nodeReadyStatus = corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	nodeNotReadyStatus = corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
	}
)

func TestReconcileGateway_electActiveEndpoint(t *testing.T) {
	mockReconciler := &ReconcileGateway{
		Configration: config.GatewayPickupControllerConfiguration{},
	}
	var tt = []struct {
		name        string
		nodeList    corev1.NodeList
		gw          *ravenv1alpha1.Gateway
		expectedEps []*ravenv1alpha1.Endpoint
	}{

		{
			// The node hosting active endpoint becomes NotReady, and it is the only node in the Gateway,
			// then the active endpoint should be removed.
			name: "lost active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					ProxyConfig: ravenv1alpha1.ProxyConfiguration{
						Replicas:        1,
						ProxyServerPort: "10262, 10263",
					},
					TunnelConfig: ravenv1alpha1.TunnelConfiguration{
						Replicas:      1,
						VPNServerPort: "4500",
					},
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1alpha1.Endpoint{},
		},
		{
			// The node hosting active endpoint becomes NotReady, but there are at least one Ready node,
			// then a new endpoint should be elected active endpoint to replace the old one.
			name: "switch active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					ProxyConfig: ravenv1alpha1.ProxyConfiguration{
						Replicas:        2,
						ProxyServerPort: "10262, 10263",
					},
					TunnelConfig: ravenv1alpha1.TunnelConfiguration{
						Replicas:      1,
						VPNServerPort: "4500",
					},
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Proxy,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1alpha1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1alpha1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1alpha1.Proxy,
				},
			},
		},

		{
			name: "elect new active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					ProxyConfig: ravenv1alpha1.ProxyConfiguration{
						Replicas:        2,
						ProxyServerPort: "10262, 10263",
					},
					TunnelConfig: ravenv1alpha1.TunnelConfiguration{
						Replicas:      1,
						VPNServerPort: "4500",
					},
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Proxy,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1alpha1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1alpha1.Proxy,
				},
				{
					NodeName: "node-3",
					Type:     ravenv1alpha1.Proxy,
				},
			},
		},

		{
			name: "no available active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeNotReadyStatus,
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-4",
						},
						Status: nodeNotReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					ProxyConfig: ravenv1alpha1.ProxyConfiguration{
						Replicas:        2,
						ProxyServerPort: "10262, 10263",
					},
					TunnelConfig: ravenv1alpha1.TunnelConfiguration{
						Replicas:      1,
						VPNServerPort: "4500",
					},
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1alpha1.Proxy,
						},
						{
							NodeName: "node-4",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1alpha1.Proxy,
						},
						{
							NodeName: "node-4",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1alpha1.Endpoint{},
		},

		{
			// The node hosting the active endpoint is still ready, do not change it.
			name: "don't switch active endpoint",
			nodeList: corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Status: nodeNotReadyStatus,
					}, {
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Status: nodeReadyStatus,
					},
				},
			},
			gw: &ravenv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1alpha1.GatewaySpec{
					ProxyConfig: ravenv1alpha1.ProxyConfiguration{
						Replicas:        1,
						ProxyServerPort: "10262, 10263",
					},
					TunnelConfig: ravenv1alpha1.TunnelConfiguration{
						Replicas:      1,
						VPNServerPort: "4500",
					},
					Endpoints: []ravenv1alpha1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
				Status: ravenv1alpha1.GatewayStatus{
					ActiveEndpoints: []*ravenv1alpha1.Endpoint{
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1alpha1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1alpha1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1alpha1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1alpha1.Proxy,
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			eps := mockReconciler.electActiveEndpoint(v.nodeList, v.gw)
			a.Equal(v.expectedEps, eps)
		})
	}

}

func TestReconcileGateway_getPodCIDRs(t *testing.T) {
	mockReconciler := &ReconcileGateway{
		Configration: config.GatewayPickupControllerConfiguration{},
	}
	var tt = []struct {
		name          string
		node          corev1.Node
		expectPodCIDR []string
	}{
		{
			name: "node has pod CIDR",
			node: corev1.Node{
				Spec: corev1.NodeSpec{
					PodCIDR: "10.0.0.1/24",
				},
			},
			expectPodCIDR: []string{"10.0.0.1/24"},
		},
		{
			name: "node hasn't pod CIDR",
			node: corev1.Node{
				Spec: corev1.NodeSpec{},
			},
			expectPodCIDR: []string{""},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			podCIDRs, err := mockReconciler.getPodCIDRs(context.Background(), v.node)
			if a.NoError(err) {
				a.Equal(v.expectPodCIDR, podCIDRs)
			}
		})
	}
}
