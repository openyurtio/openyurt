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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypickup/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	ProxyIP      = "172.168.0.1"
	Node1Name    = "node-1"
	Node2Name    = "node-2"
	Node3Name    = "node-3"
	Node4Name    = "node-4"
	Node1Address = "192.168.0.1"
	Node2Address = "192.168.0.2"
	Node3Address = "192.168.0.3"
	Node4Address = "192.168.0.4"
	MockGateway  = "gw-mock"
	MockProxySvc = "x-raven-proxy-internal-svc"
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

func MockReconcile() *ReconcileGateway {
	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node1Name,
					Labels: map[string]string{
						raven.LabelCurrentGateway: MockGateway,
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node1Address,
						},
					},
					Conditions: []corev1.NodeCondition{
						{
							Type: "Ready",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node2Name,
					Labels: map[string]string{
						raven.LabelCurrentGateway: MockGateway,
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node2Address,
						},
					},
				},
			},
		},
	}
	configmaps := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RavenGlobalConfig,
					Namespace: util.WorkingNamespace,
				},
				Data: map[string]string{
					util.RavenEnableProxy:  "true",
					util.RavenEnableTunnel: "true",
				},
			},
		},
	}
	gateways := &ravenv1beta1.GatewayList{
		Items: []ravenv1beta1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MockGateway,
					Namespace: util.WorkingNamespace,
				},
				Spec: ravenv1beta1.GatewaySpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							raven.LabelCurrentGateway: MockGateway,
						},
					},
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas:       2,
						ProxyHTTPPort:  "10266,10267,10255,9100",
						ProxyHTTPSPort: "10250,9445",
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: Node1Name,
							Type:     ravenv1beta1.Proxy,
							UnderNAT: false,
						},
						{
							NodeName: Node2Name,
							Type:     ravenv1beta1.Proxy,
							UnderNAT: false,
						},
					},
					ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
				},
				Status: ravenv1beta1.GatewayStatus{
					Nodes: []ravenv1beta1.NodeInfo{
						{
							NodeName:  Node1Name,
							PrivateIP: Node1Address,
						},
						{
							NodeName:  Node2Name,
							PrivateIP: Node2Address,
						},
					},
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: Node1Name,
							Type:     ravenv1beta1.Proxy,
							UnderNAT: false,
						},
						{
							NodeName: Node2Name,
							Type:     ravenv1beta1.Proxy,
							UnderNAT: false,
						},
					},
				},
			},
		},
	}
	objs := []runtime.Object{nodeList, gateways, configmaps}
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil
	}
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil
	}
	status := []client.Object{&gateways.Items[0]}

	return &ReconcileGateway{
		Client:        fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(status...).Build(),
		recorder:      record.NewFakeRecorder(100),
		Configuration: config.GatewayPickupControllerConfiguration{},
	}
}

func TestReconcileGateway_Reconcile(t *testing.T) {
	r := MockReconcile()
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: MockGateway, Namespace: util.WorkingNamespace}})
	if err != nil {
		t.Errorf("failed to reconcile gateway %s/%s", util.WorkingNamespace, util.GatewayProxyInternalService)
	}
}

func TestReconcileGateway_electActiveEndpoint(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.RavenGlobalConfig,
			Namespace: util.WorkingNamespace,
		},
		Data: map[string]string{
			util.RavenEnableProxy:  "true",
			util.RavenEnableTunnel: "true",
		},
	}

	mockReconciler := &ReconcileGateway{
		Configuration: config.GatewayPickupControllerConfiguration{},
		Client:        fake.NewClientBuilder().WithObjects(obj).Build(),
	}
	var tt = []struct {
		name        string
		nodeList    corev1.NodeList
		gw          *ravenv1beta1.Gateway
		expectedEps []*ravenv1beta1.Endpoint
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
			gw: &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 1,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
				Status: ravenv1beta1.GatewayStatus{
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1beta1.Endpoint{},
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
			gw: &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 2,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Proxy,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
				Status: ravenv1beta1.GatewayStatus{
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
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
			gw: &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 2,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Proxy,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
				Status: ravenv1beta1.GatewayStatus{
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
				},
				{
					NodeName: "node-3",
					Type:     ravenv1beta1.Proxy,
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
			gw: &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 2,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1beta1.Proxy,
						},
						{
							NodeName: "node-4",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
				Status: ravenv1beta1.GatewayStatus{
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-3",
							Type:     ravenv1beta1.Proxy,
						},
						{
							NodeName: "node-4",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1beta1.Endpoint{},
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
			gw: &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-1",
				},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 1,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: []ravenv1beta1.Endpoint{
						{
							NodeName: "node-1",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
				Status: ravenv1beta1.GatewayStatus{
					ActiveEndpoints: []*ravenv1beta1.Endpoint{
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Tunnel,
						},
						{
							NodeName: "node-2",
							Type:     ravenv1beta1.Proxy,
						},
					},
				},
			},
			expectedEps: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
				},
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			eps := mockReconciler.electActiveEndpoint(v.nodeList, v.gw)
			a.Equal(len(v.expectedEps), len(eps))
		})
	}

}

func TestReconcileGateway_getPodCIDRs(t *testing.T) {
	mockReconciler := &ReconcileGateway{
		Configuration: config.GatewayPickupControllerConfiguration{},
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

func TestReconcileGateway_configEndpoints(t *testing.T) {
	ctx := context.TODO()

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.RavenGlobalConfig,
			Namespace: util.WorkingNamespace,
		},
		Data: map[string]string{
			util.RavenEnableProxy:  "true",
			util.RavenEnableTunnel: "true",
		},
	}

	mockReconciler := &ReconcileGateway{
		Configuration: config.GatewayPickupControllerConfiguration{},
		Client:        fake.NewClientBuilder().WithObjects(obj).Build(),
	}

	gw := &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway-1",
		},
		Spec: ravenv1beta1.GatewaySpec{
			ProxyConfig: ravenv1beta1.ProxyConfiguration{
				Replicas: 1,
			},
			TunnelConfig: ravenv1beta1.TunnelConfiguration{
				Replicas: 1,
			},
			Endpoints: []ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
				},
			},
		},
		Status: ravenv1beta1.GatewayStatus{
			ActiveEndpoints: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
				},
			},
		},
	}

	expect := &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway-1",
		},
		Spec: ravenv1beta1.GatewaySpec{
			ProxyConfig: ravenv1beta1.ProxyConfiguration{
				Replicas: 1,
			},
			TunnelConfig: ravenv1beta1.TunnelConfiguration{
				Replicas: 1,
			},
			Endpoints: []ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
				},
			},
		},
		Status: ravenv1beta1.GatewayStatus{
			ActiveEndpoints: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
					Config: map[string]string{
						util.RavenEnableTunnel: "true",
					},
				},
				{
					NodeName: "node-2",
					Type:     ravenv1beta1.Proxy,
					Config: map[string]string{
						util.RavenEnableProxy: "true",
					},
				},
			},
		},
	}

	mockReconciler.configEndpoints(ctx, gw)

	if !reflect.DeepEqual(gw.Status.ActiveEndpoints[0].Config, expect.Status.ActiveEndpoints[0].Config) {
		t.Errorf("Expected %v to be present in the first endpoint config, but get %sv", expect.Status.ActiveEndpoints[0].Config, gw.Status.ActiveEndpoints[0].Config)
	}

	if !reflect.DeepEqual(gw.Status.ActiveEndpoints[1].Config, expect.Status.ActiveEndpoints[1].Config) {
		t.Errorf("Expected %v to be present in the second endpoint config, but get %v", expect.Status.ActiveEndpoints[1].Config, gw.Status.ActiveEndpoints[1].Config)
	}
}

func TestReconcileGateway_addExtraAllowedSubnet(t *testing.T) {
	mockReconciler := &ReconcileGateway{}
	gw := &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway",
			Annotations: map[string]string{
				util.ExtraAllowedSourceCIDRs: "1.1.1.1/32,2.2.2.2/32",
			},
		},
		Spec: ravenv1beta1.GatewaySpec{
			TunnelConfig: ravenv1beta1.TunnelConfiguration{
				Replicas: 1,
			},
			Endpoints: []ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
			},
		},
		Status: ravenv1beta1.GatewayStatus{
			ActiveEndpoints: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
			},
			Nodes: []ravenv1beta1.NodeInfo{
				{
					NodeName:  "node-1",
					PrivateIP: "10.10.10.10",
					Subnets:   []string{"10.244.10.0/24"},
				},
			},
		},
	}
	expect := &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway",
			Annotations: map[string]string{
				util.ExtraAllowedSourceCIDRs: "1.1.1.1/32,2.2.2.2/32",
			},
		},
		Spec: ravenv1beta1.GatewaySpec{
			TunnelConfig: ravenv1beta1.TunnelConfiguration{
				Replicas: 1,
			},
			Endpoints: []ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
			},
		},
		Status: ravenv1beta1.GatewayStatus{
			ActiveEndpoints: []*ravenv1beta1.Endpoint{
				{
					NodeName: "node-1",
					Type:     ravenv1beta1.Tunnel,
				},
			},
			Nodes: []ravenv1beta1.NodeInfo{
				{
					NodeName:  "node-1",
					PrivateIP: "10.10.10.10",
					Subnets:   []string{"10.244.10.0/24", "1.1.1.1/32", "2.2.2.2/32"},
				},
			},
		},
	}
	mockReconciler.addExtraAllowedSubnet(gw)
	if !reflect.DeepEqual(gw.Status.Nodes, expect.Status.Nodes) {
		t.Errorf("failed add extra allowed subnet, expect %v, but get %v", expect.Status.Nodes, gw.Status.Nodes)
	}
}
