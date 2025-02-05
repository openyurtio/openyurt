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

package hubleaderconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderconfig/config"
)

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()

	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = apis.AddToScheme(scheme)
	require.NoError(t, err)

	testCases := map[string]struct {
		pool              *appsv1beta2.NodePool
		existingConfigMap *v1.ConfigMap
		expectedConfigMap *v1.ConfigMap
		expectErr         bool
	}{
		"one endpoint": {
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
					PoolScopeMetadata: []metav1.GroupVersionKind{
						{
							Group:   "core",
							Version: "v1",
							Kind:    "Service",
						},
						{
							Group:   "discovery.k8s.io",
							Version: "v1",
							Kind:    "EndpointSlice",
						},
					},
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "node1",
							Address:  "10.0.0.1",
						},
					},
				},
			},
			existingConfigMap: nil,
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leader-hub-hangzhou",
				},
				Data: map[string]string{
					"leaders":              "node1/10.0.0.1",
					"pool-scoped-metadata": "core/v1/Service,discovery.k8s.io/v1/EndpointSlice",
				},
			},
			expectErr: false,
		},
		"multiple endpoints": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
					PoolScopeMetadata: []metav1.GroupVersionKind{
						{
							Group:   "core",
							Version: "v1",
							Kind:    "Service",
						},
						{
							Group:   "discovery.k8s.io",
							Version: "v1",
							Kind:    "EndpointSlice",
						},
					},
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "node1",
							Address:  "10.0.0.1",
						},
						{
							NodeName: "node2",
							Address:  "10.0.0.2",
						},
					},
				},
			},
			existingConfigMap: nil,
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leader-hub-shanghai",
				},
				Data: map[string]string{
					"leaders":              "node1/10.0.0.1,node2/10.0.0.2",
					"pool-scoped-metadata": "core/v1/Service,discovery.k8s.io/v1/EndpointSlice",
				},
			},
			expectErr: false,
		},
		"config map need update": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "shanghai",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
					PoolScopeMetadata: []metav1.GroupVersionKind{
						{
							Group:   "core",
							Version: "v1",
							Kind:    "Service",
						},
						{
							Group:   "discovery.k8s.io",
							Version: "v1",
							Kind:    "EndpointSlice",
						},
					},
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "node1",
							Address:  "10.0.0.1",
						},
						{
							NodeName: "node2",
							Address:  "10.0.0.2",
						},
					},
				},
			},
			existingConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leader-hub-shanghai",
				},
				Data: map[string]string{
					"leaders":              "node1/10.0.0.1",
					"pool-scoped-metadata": "core/v1/Service",
				},
			},
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leader-hub-shanghai",
				},
				Data: map[string]string{
					"leaders":              "node1/10.0.0.1,node2/10.0.0.2",
					"pool-scoped-metadata": "core/v1/Service,discovery.k8s.io/v1/EndpointSlice",
				},
			},
			expectErr: false,
		},
		"no endpoints": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "beijing",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
					PoolScopeMetadata: []metav1.GroupVersionKind{
						{
							Group:   "core",
							Version: "v1",
							Kind:    "Service",
						},
						{
							Group:   "discovery.k8s.io",
							Version: "v1",
							Kind:    "EndpointSlice",
						},
					},
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{},
				},
			},
			existingConfigMap: nil,
			expectedConfigMap: nil,
			expectErr:         false,
		},
		"no pool scope metadata": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "beijing",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      true,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "node1",
							Address:  "10.0.0.1",
						},
						{
							NodeName: "node2",
							Address:  "10.0.0.2",
						},
					},
				},
			},
			existingConfigMap: nil,
			expectedConfigMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leader-hub-beijing",
				},
				Data: map[string]string{
					"leaders":              "node1/10.0.0.1,node2/10.0.0.2",
					"pool-scoped-metadata": "",
				},
			},
			expectErr: false,
		},
		"no interconnectivity": {
			pool: &appsv1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing",
				},
				Spec: appsv1beta2.NodePoolSpec{
					Type: appsv1beta2.Edge,
					Labels: map[string]string{
						"region": "beijing",
					},
					LeaderReplicas:         1,
					LeaderElectionStrategy: string(appsv1beta2.ElectionStrategyRandom),
					InterConnectivity:      false,
				},
				Status: appsv1beta2.NodePoolStatus{
					LeaderEndpoints: []appsv1beta2.Leader{
						{
							NodeName: "node1",
							Address:  "10.0.0.1",
						},
						{
							NodeName: "node2",
							Address:  "10.0.0.2",
						},
					},
				},
			},
			existingConfigMap: nil,
			expectedConfigMap: nil,
			expectErr:         false,
		},
	}

	ctx := context.TODO()
	for k, tc := range testCases {
		t.Run(k, func(t *testing.T) {
			c := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.pool).
				WithStatusSubresource(tc.pool)

			// Add existing ConfigMap if it exists
			if tc.existingConfigMap != nil {
				c.WithObjects(tc.existingConfigMap)
			}

			r := &ReconcileHubLeaderConfig{
				Client:        c.Build(),
				Configuration: config.HubLeaderConfigControllerConfiguration{},
				recorder:      record.NewFakeRecorder(1000),
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: tc.pool.Name}}
			_, err := r.Reconcile(ctx, req)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var actualConfig v1.ConfigMap
			if tc.expectedConfigMap == nil {
				err = r.Get(ctx, types.NamespacedName{
					Name:      "leader-hub-" + tc.pool.Name,
					Namespace: tc.pool.Namespace,
				}, &actualConfig)
				require.True(t, errors.IsNotFound(err))
				return
			}

			err = r.Get(ctx, types.NamespacedName{
				Name:      tc.expectedConfigMap.Name,
				Namespace: tc.expectedConfigMap.Namespace,
			}, &actualConfig)
			require.NoError(t, err)

			// Reset resource version - it's not important for the test
			actualConfig.ResourceVersion = ""

			require.Equal(t, *tc.expectedConfigMap, actualConfig)
		})
	}
}
