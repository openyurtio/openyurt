/*
Copyright 2023 The OpenYurt Authors.

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

package nodepool

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	poolconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodepool/config"
)

func prepareNodes() []client.Object {
	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "hangzhou",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "hangzhou",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeNetworkUnavailable,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "beijing",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node4",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "beijing",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}
	return nodes
}

func prepareNodePools() []client.Object {
	pools := []client.Object{
		&appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hangzhou",
			},
			Spec: appsv1beta1.NodePoolSpec{
				Type: appsv1beta1.Edge,
				Labels: map[string]string{
					"region": "hangzhou",
				},
			},
		},
		&appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "beijing",
			},
			Spec: appsv1beta1.NodePoolSpec{
				Type: appsv1beta1.Edge,
				Labels: map[string]string{
					"region": "beijing",
				},
			},
		},
		&appsv1beta1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "shanghai",
			},
			Spec: appsv1beta1.NodePoolSpec{
				Type: appsv1beta1.Edge,
				Labels: map[string]string{
					"region": "shanghai",
				},
			},
		},
	}
	return pools
}

func TestReconcile(t *testing.T) {
	nodes := prepareNodes()
	pools := prepareNodePools()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}
	apis.AddToScheme(scheme)

	c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(pools...).WithObjects(nodes...).Build()
	testcases := map[string]struct {
		EnableSyncNodePoolConfigurations bool
		pool                             string
		wantedPool                       *appsv1beta1.NodePool
		wantedNodes                      []corev1.Node
		err                              error
	}{
		"reconcile hangzhou pool": {
			pool: "hangzhou",
			wantedPool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta1.NodePoolSpec{
					Type: appsv1beta1.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
				},
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   1,
					UnreadyNodeNum: 1,
					Nodes:          []string{"node1", "node2"},
				},
			},
		},
		"reconcile beijing pool": {
			EnableSyncNodePoolConfigurations: true,
			pool:                             "beijing",
			wantedPool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing",
				},
				Spec: appsv1beta1.NodePoolSpec{
					Type: appsv1beta1.Edge,
					Labels: map[string]string{
						"region": "beijing",
					},
				},
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   1,
					UnreadyNodeNum: 1,
					Nodes:          []string{"node3", "node4"},
				},
			},
			wantedNodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "beijing",
							"region":                       "beijing",
						},
						Annotations: map[string]string{
							"nodepool.openyurt.io/previous-attributes": "{\"labels\":{\"region\":\"beijing\"}}",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "beijing",
							"region":                       "beijing",
						},
						Annotations: map[string]string{
							"nodepool.openyurt.io/previous-attributes": "{\"labels\":{\"region\":\"beijing\"}}",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		"reconcile shanghai pool without nodes": {
			pool: "shanghai",
			wantedPool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta1.NodePoolSpec{
					Type: appsv1beta1.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
				},
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
				},
			},
		},
	}

	ctx := context.TODO()
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			r := &ReconcileNodePool{
				Client: c,
				cfg: poolconfig.NodePoolControllerConfiguration{
					EnableSyncNodePoolConfigurations: tc.EnableSyncNodePoolConfigurations,
				},
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: tc.pool}}
			_, err := r.Reconcile(ctx, req)
			if err != nil {
				t.Errorf("Reconcile() error = %v", err)
				return
			}

			var wantedPool appsv1beta1.NodePool
			if err := r.Get(ctx, req.NamespacedName, &wantedPool); err != nil {
				t.Errorf("Reconcile() error = %v", err)
				return
			}
			if !reflect.DeepEqual(wantedPool.Status, tc.wantedPool.Status) {
				t.Errorf("expected %#+v, got %#+v", tc.wantedPool.Status, wantedPool.Status)
				return
			}

			if len(tc.wantedNodes) != 0 {
				var currentNodeList corev1.NodeList
				if err := r.List(ctx, &currentNodeList, client.MatchingLabels(map[string]string{
					projectinfo.GetNodePoolLabel(): tc.pool,
				})); err != nil {
					t.Errorf("Reconcile() error = %v", err)
					return
				}
				gotNodes := make([]corev1.Node, 0)
				for i := range currentNodeList.Items {
					node := currentNodeList.Items[i]
					node.ObjectMeta.ResourceVersion = ""
					gotNodes = append(gotNodes, node)
				}

				if !reflect.DeepEqual(tc.wantedNodes, gotNodes) {
					t.Errorf("expected %#+v, \ngot %#+v", tc.wantedNodes, gotNodes)
				}
			}
		})
	}
}
