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

package hubleaderrbac

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderrbac/config"
)

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()

	err := clientgoscheme.AddToScheme(scheme)
	require.NoError(t, err)
	err = apis.AddToScheme(scheme)
	require.NoError(t, err)

	testCases := map[string]struct {
		pools               *appsv1beta2.NodePoolList
		existingClusterRole *v1.ClusterRole
		expectedClusterRole *v1.ClusterRole
		expectErr           bool
	}{
		"no pool scoped metadata": {
			pools: &appsv1beta2.NodePoolList{
				Items: []appsv1beta2.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hangzhou",
						},
						Spec: appsv1beta2.NodePoolSpec{
							Type: appsv1beta2.Edge,
							Labels: map[string]string{
								"region": "hangzhou",
							},
						},
					},
				},
			},
			existingClusterRole: nil,
			expectedClusterRole: &v1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: leaderRoleName,
				},
				Rules: []v1.PolicyRule{},
			},
		},
		"pool scoped metadata": {
			pools: &appsv1beta2.NodePoolList{
				Items: []appsv1beta2.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hangzhou",
						},
						Spec: appsv1beta2.NodePoolSpec{
							Type: appsv1beta2.Edge,
							Labels: map[string]string{
								"region": "hangzhou",
							},
							PoolScopeMetadata: []metav1.GroupVersionResource{
								{
									Group:    "",
									Resource: "services",
									Version:  "v1",
								},
								{
									Group:    "discovery.k8s.io",
									Resource: "endpointslices",
									Version:  "v1",
								},
							},
						},
					},
				},
			},
			existingClusterRole: nil,
			expectedClusterRole: &v1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: leaderRoleName,
				},
				Rules: []v1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"services"},
						Verbs:     []string{"list", "watch"},
					},
					{
						APIGroups: []string{"discovery.k8s.io"},
						Resources: []string{"endpointslices"},
						Verbs:     []string{"list", "watch"},
					},
				},
			},
		},
		"multiple nodepools": {
			pools: &appsv1beta2.NodePoolList{
				Items: []appsv1beta2.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hangzhou",
						},
						Spec: appsv1beta2.NodePoolSpec{
							Type: appsv1beta2.Edge,
							Labels: map[string]string{
								"region": "hangzhou",
							},
							PoolScopeMetadata: []metav1.GroupVersionResource{
								{
									Group:    "",
									Resource: "services",
									Version:  "v1",
								},
								{
									Group:    "discovery.k8s.io",
									Resource: "endpointslices",
									Version:  "v1",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "shanghai",
						},
						Spec: appsv1beta2.NodePoolSpec{
							Type: appsv1beta2.Edge,
							Labels: map[string]string{
								"region": "shanghai",
							},
							PoolScopeMetadata: []metav1.GroupVersionResource{
								{
									Group:    "",
									Resource: "services",
									Version:  "v1",
								},
								{
									Group:    "discovery.k8s.io",
									Resource: "endpoints",
									Version:  "v1",
								},
							},
						},
					},
				},
			},
			existingClusterRole: nil,
			expectedClusterRole: &v1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: leaderRoleName,
				},
				Rules: []v1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"services"},
						Verbs:     []string{"list", "watch"},
					},
					{
						APIGroups: []string{"discovery.k8s.io"},
						Resources: []string{"endpointslices", "endpoints"},
						Verbs:     []string{"list", "watch"},
					},
				},
			},
		},
		"pool scoped metadata added": {
			pools: &appsv1beta2.NodePoolList{
				Items: []appsv1beta2.NodePool{
					{

						ObjectMeta: metav1.ObjectMeta{
							Name: "hangzhou",
						},
						Spec: appsv1beta2.NodePoolSpec{
							Type: appsv1beta2.Edge,
							Labels: map[string]string{
								"region": "hangzhou",
							},
							PoolScopeMetadata: []metav1.GroupVersionResource{
								{
									Group:    "discovery.k8s.io",
									Resource: "endpointslices",
									Version:  "v1",
								},
								{
									Group:    "",
									Resource: "services",
									Version:  "v1",
								},
							},
						},
					},
				},
			},
			existingClusterRole: &v1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: leaderRoleName,
				},
				Rules: []v1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"services"},
						Verbs:     []string{"list", "watch"},
					},
				},
			},
			expectedClusterRole: &v1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: leaderRoleName,
				},
				Rules: []v1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"services"},
						Verbs:     []string{"list", "watch"},
					},
					{
						APIGroups: []string{"discovery.k8s.io"},
						Resources: []string{"endpointslices"},
						Verbs:     []string{"list", "watch"},
					},
				},
			},
		},
	}

	ctx := context.TODO()
	for k, tc := range testCases {
		t.Run(k, func(t *testing.T) {
			c := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithLists(tc.pools)

			// Add existing cluster role if it exists
			if tc.existingClusterRole != nil {
				c.WithObjects(tc.existingClusterRole)
			}

			r := &ReconcileHubLeaderRBAC{
				Client:        c.Build(),
				Configuration: config.HubLeaderRBACControllerConfiguration{},
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: tc.pools.Items[0].Name}}
			_, err := r.Reconcile(ctx, req)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var actualClusterRole v1.ClusterRole
			err = r.Get(ctx, types.NamespacedName{
				Name: leaderRoleName,
			}, &actualClusterRole)

			if tc.expectedClusterRole == nil {
				require.True(t, errors.IsNotFound(err))
				return
			}

			// Reset resource version - it's not important for the test
			actualClusterRole.ResourceVersion = ""
			assert.Equal(t, *tc.expectedClusterRole, actualClusterRole)
		})
	}
}

func TestHasPolicyChanged(t *testing.T) {
	testCases := map[string]struct {
		oldRules []v1.PolicyRule
		newRules []v1.PolicyRule
		expected bool
	}{
		"no change": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			expected: false,
		},
		"out of order verbs": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"list", "get"},
				},
			},
			expected: false,
		},
		"out of order groups": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{"discovery.k8s.io", ""},
					Resources: []string{"endpointslices"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{"", "discovery.k8s.io"},
					Resources: []string{"endpointslices"},
					Verbs:     []string{"get", "list"},
				},
			},
			expected: false,
		},
		"out of order resources": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"endpointslices", "services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services", "endpointslices"},
					Verbs:     []string{"get", "list"},
				},
			},
			expected: false,
		},
		"changed api group": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			expected: true,
		},
		"changed resources": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"nodes"},
					Verbs:     []string{"get", "list"},
				},
			},
			expected: true,
		},
		"changed verbs": {
			oldRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "list"},
				},
			},
			newRules: []v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services"},
					Verbs:     []string{"get", "watch"},
				},
			},
			expected: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := hasPolicyChanged(tc.oldRules, tc.newRules)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
