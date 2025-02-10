/*
Copyright 2025 The OpenYurt Authors.

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

package v1beta2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
)

func TestDefault(t *testing.T) {
	testcases := map[string]struct {
		obj            runtime.Object
		expectErr      bool
		wantedNodePool *v1beta2.NodePool
	}{
		"it is not a nodepool": {
			obj:       &corev1.Pod{},
			expectErr: true,
		},
		"nodepool has no type": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:    true,
					LeaderReplicas: 3,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"nodepool.openyurt.io/type": "edge",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Edge,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyRandom),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has pool type": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:    true,
					Type:           v1beta2.Cloud,
					LeaderReplicas: 3,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyRandom),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has no leader election strategy": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: "",
					LeaderReplicas:         3,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyRandom),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has no mark election strategy": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has no pool scope metadata": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has pool scope metadata": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "Endpoints",
						},
					},
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "Endpoints",
						},
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has v1.service pool scope metadata": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
					},
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has v1.endpointslices pool scope metadata": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         3,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has leader replicas": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         2,
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         2,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has no leader replicas": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         0,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
					},
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:            true,
					Type:                   v1beta2.Cloud,
					LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:         1,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool doesn't have enable scope metadata": {
			obj: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:             true,
					Type:                    v1beta2.Cloud,
					LeaderElectionStrategy:  string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:          3,
					EnablePoolScopeMetadata: false,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "Endpoints",
						},
					},
				},
			},
			wantedNodePool: &v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"foo":                       "bar",
						"nodepool.openyurt.io/type": "cloud",
					},
				},
				Spec: v1beta2.NodePoolSpec{
					HostNetwork:             true,
					Type:                    v1beta2.Cloud,
					LeaderElectionStrategy:  string(v1beta2.ElectionStrategyMark),
					LeaderReplicas:          3,
					EnablePoolScopeMetadata: false,
					PoolScopeMetadata: []metav1.GroupVersionResource{
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "Endpoints",
						},
						{
							Group:    "core",
							Version:  "v1",
							Resource: "services",
						},
						{
							Group:    "discovery.k8s.io",
							Version:  "v1",
							Resource: "endpointslices",
						},
					},
				},
				Status: v1beta2.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := NodePoolHandler{}
			err := h.Default(context.TODO(), tc.obj)
			if tc.expectErr {
				require.Error(t, err, "expected no error")
				return
			}
			require.NoError(t, err, "expected error")

			currentNp := tc.obj.(*v1beta2.NodePool)
			assert.Equal(t, tc.wantedNodePool, currentNp)
		})
	}
}
