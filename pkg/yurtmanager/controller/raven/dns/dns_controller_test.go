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

package dns

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1v1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
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

func mockKubeClient() client.Client {
	nodeList := &v1.NodeList{
		Items: []v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node1Name,
					Labels: map[string]string{
						raven.LabelCurrentGateway: MockGateway,
					},
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
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
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: Node2Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node3Name,
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: Node3Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node4Name,
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: Node4Address,
						},
					},
				},
			},
		},
	}

	services := &v1.ServiceList{
		Items: []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MockProxySvc,
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:     MockGateway,
						raven.LabelCurrentGatewayType: ravenv1v1beta1.Proxy,
					},
				},
				Spec: v1.ServiceSpec{
					Type:      v1.ServiceTypeClusterIP,
					ClusterIP: ProxyIP,
					Ports:     []v1.ServicePort{},
				},
			},
		},
	}

	configmaps := &v1.ConfigMapList{
		Items: []v1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RavenProxyNodesConfig,
					Namespace: util.WorkingNamespace,
				},
				Data: map[string]string{
					util.ProxyNodesKey: "",
				},
			},
		},
	}
	objs := []runtime.Object{nodeList, configmaps, services}
	return fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
}

func mockReconciler() *ReconcileDns {
	return &ReconcileDns{
		Client:   mockKubeClient(),
		recorder: record.NewFakeRecorder(100),
	}
}

func TestReconcileDns_Reconcile(t *testing.T) {
	r := mockReconciler()
	t.Run("get dns configmap", func(t *testing.T) {
		res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenProxyNodesConfig}})
		assert.Equal(t, reconcile.Result{}, res)
		assert.Equal(t, err, nil)
	})
}
