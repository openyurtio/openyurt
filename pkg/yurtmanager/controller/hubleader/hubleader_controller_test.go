/*
Copyright 2025 The OpenYurt Authors.

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

package hubleader

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleader/config"
)

// prepareNodes returns a list of nodes for testing
// 5 nodes are in hangzhou are in various readiness statuses. Only 1 will be a valid leader for random election strategy.
// 4 nodes are in shanghai. 2 are valid candidates for leader election. 1 isn't marked and the other isn't ready.
// For deterministic test results, hangzhou is used for random election strategy
// and shanghai is used for mark election strategy.
func prepareNodes() []client.Object {
	return []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ready no internal IP",
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
				Name: "not ready no internal IP",
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
				Name: "not ready internal IP",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "hangzhou",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					},
				},
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
				Name: "no condition",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "hangzhou",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ready with internal IP",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "hangzhou",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.1",
					},
				},
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
				Name: "ready with internal IP and marked as leader",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "shanghai",
					"apps.openyurt.io/leader":      "true",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.2",
					},
				},
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
				Name: "ready with internal IP and not marked as leader",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "shanghai",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.3",
					},
				},
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
				Name: "not ready with internal IP marked as leader",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "shanghai",
					"apps.openyurt.io/leader":      "true",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.4",
					},
				},
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
				Name: "ready with internal IP and marked as 2nd leader",
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel(): "shanghai",
					"apps.openyurt.io/leader":      "true",
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "10.0.0.5",
					},
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}
}

func TestReconcile(t *testing.T) {
	nodes := prepareNodes()
	scheme := runtime.NewScheme()

	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = apis.AddToScheme(scheme)
	require.NoError(t, err)

	testCases := map[string]struct {
		pool             *appsv1beta2.NodePool
		expectedNodePool *appsv1beta2.NodePool
		expectErr        bool
	}{
		"random election strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP",
							Endpoint: "10.0.0.1",
						},
					},
				},
			},
			expectErr: false,
		},
		"mark election strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					LeaderReplicas:    2,
					InterConnectivity: true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
					LeaderReplicas:    2,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "ready with internal IP and marked as 2nd leader",
							Endpoint: "10.0.0.5",
						},
					},
				},
			},
			expectErr: false,
		},
		"no potential leaders in hangzhou with mark strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou", // there are no marked leaders in hangzhou
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
			},
			expectErr: false,
		},
		"interconnectivity false with mark strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: false, // should not change nodepool
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: false,
				},
			},
			expectErr: false,
		},
		"interconnectivity false with random strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: false, // should not change nodepool
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: false,
				},
			},
			expectErr: false,
		},
		"invalid election strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: "", // invalid strategy
					InterConnectivity:      true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "hangzhou",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: "",
					InterConnectivity:      true,
				},
			},
			expectErr: true,
		},
		"no election required with mark strategy": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					LeaderReplicas:    1, // set to 1 as there's 2 possible leaders in pool
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2", // leader already set
						},
					},
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					LeaderReplicas:    1,
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2", // should not change leader as replicas met
						},
					},
				},
			},
			expectErr: false,
		},
		"re election required": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "not ready with internal IP marked as leader",
							Endpoint: "10.0.0.4", // .4 was leader (node not ready)
						},
					},
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "ready with internal IP and marked as 2nd leader",
							Endpoint: "10.0.0.5", // new leader is .5
						},
					},
				},
			},
			expectErr: false,
		},
		"mark strategy multiple leaders": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         3, // higher than number of available leaders
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "ready with internal IP and marked as 2nd leader",
							Endpoint: "10.0.0.5",
						}, // multiple marked leaders
					},
				},
			},
			expectErr: false,
		},
		"random strategy multiple leaders": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					LeaderReplicas:         3, // higher than number of available leaders
					InterConnectivity:      true,
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					LeaderReplicas:         3,
					InterConnectivity:      true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "ready with internal IP and not marked as leader",
							Endpoint: "10.0.0.3",
						},
						{
							NodeName: "ready with internal IP and marked as 2nd leader",
							Endpoint: "10.0.0.5",
						}, // multiple marked leaders,
					},
				},
			},
			expectErr: false,
		},
		"leader replicas reduced": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         1, // Nodepool leader replicas reduced
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
						{
							NodeName: "ready with internal IP and marked as 2nd leader",
							Endpoint: "10.0.0.5",
						}, // 2 leaders set, last should be dropped
					},
				},
			},
			expectedNodePool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyMark),
					LeaderReplicas:         1,
					LeaderNodeLabelSelector: map[string]string{
						"apps.openyurt.io/leader": "true",
					},
					InterConnectivity: true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "ready with internal IP and marked as leader",
							Endpoint: "10.0.0.2",
						},
					},
				},
			},
			expectErr: false,
		},
	}

	ctx := context.TODO()
	for k, tc := range testCases {
		t.Run(k, func(t *testing.T) {
			c := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.pool).
				WithStatusSubresource(tc.pool).
				WithObjects(nodes...).
				Build()

			r := &ReconcileHubLeader{
				Client:        c,
				Configuration: config.HubLeaderControllerConfiguration{},
				recorder:      record.NewFakeRecorder(1000),
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: tc.pool.Name}}
			_, err := r.Reconcile(ctx, req)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var actualPool appsv1beta2.NodePool
			err = r.Get(ctx, req.NamespacedName, &actualPool)
			require.NoError(t, err)

			// Reset resource version - it's not important for the test
			actualPool.ResourceVersion = ""
			// Sort leader endpoints for comparison - it is not important for the order
			slices.SortStableFunc(actualPool.Status.LeaderEndpoints, func(a, b appsv1beta2.Leader) int {
				return cmp.Compare(
					a.Endpoint,
					b.Endpoint,
				)
			})

			require.Equal(t, *tc.expectedNodePool, actualPool)
		})
	}
}
