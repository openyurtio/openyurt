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

package gatewayinternalservice

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	Node1Name    = "node-1"
	Node2Name    = "node-2"
	Node3Name    = "node-3"
	Node4Name    = "node-4"
	Node1Address = "192.168.0.1"
	Node2Address = "192.168.0.2"
	Node3Address = "192.168.0.3"
	Node4Address = "192.168.0.4"
	MockGateway  = "gw-mock"
)

func MockReconcile() *ReconcileService {
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
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node3Name,
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node3Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node4Name,
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node4Address,
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
					Name: MockGateway,
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

	return &ReconcileService{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
		recorder: record.NewFakeRecorder(100),
	}
}

func TestReconcileService_Reconcile(t *testing.T) {
	r := MockReconcile()
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: util.GatewayProxyInternalService, Namespace: util.WorkingNamespace}})
	if err != nil {
		t.Errorf("failed to reconcile service %s/%s", util.WorkingNamespace, util.GatewayProxyInternalService)
	}
}
