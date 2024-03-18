/*
Copyright 2024 The OpenYurt Authors.

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

package initializer

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type nopNodeHandler struct {
	name               string
	enablePoolTopology bool
	nodesGetter        filter.NodesInPoolGetter
	nodesSynced        cache.InformerSynced
}

func (nop *nopNodeHandler) Name() string {
	return nop.name
}

func (nop *nopNodeHandler) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{}
}

func (nop *nopNodeHandler) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	return obj
}

func (nop *nopNodeHandler) SetNodesGetterAndSynced(nodesGetter filter.NodesInPoolGetter, nodesSynced cache.InformerSynced, enablePoolTopology bool) error {
	nop.nodesGetter = nodesGetter
	nop.nodesSynced = nodesSynced
	nop.enablePoolTopology = enablePoolTopology
	return nil
}

func TestNodesInitializer(t *testing.T) {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1beta1", Resource: "nodepools"}: "NodePoolList",
	}
	nodeBucketGVRToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1alpha1", Resource: "nodebuckets"}: "NodeBucketList",
	}

	testcases := map[string]struct {
		enableNodePool            bool
		enablePoolServiceTopology bool
		poolName                  string
		yurtClient                *fake.FakeDynamicClient
		expectedNodes             sets.String
		expectedErr               bool
	}{
		"get nodes in nodepool": {
			enableNodePool:            true,
			enablePoolServiceTopology: false,
			poolName:                  "hangzhou",
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
				&v1beta1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
					},
					Spec: v1beta1.NodePoolSpec{
						Type: v1beta1.Edge,
					},
					Status: v1beta1.NodePoolStatus{
						Nodes: []string{
							"node1",
							"node2",
							"node3",
						},
					},
				},
			),
			expectedNodes: sets.NewString("node1", "node2", "node3"),
		},
		"get nodes in nodebucket": {
			enableNodePool:            true,
			enablePoolServiceTopology: true,
			poolName:                  "hangzhou",
			yurtClient: fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind,
				&v1alpha1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []v1alpha1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node2",
						},
					},
				},
			),
			expectedNodes: sets.NewString("node1", "node2"),
		},
		"nodepool doesn't exist": {
			enableNodePool:            true,
			enablePoolServiceTopology: false,
			poolName:                  "hangzhou",
			yurtClient:                fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind),
			expectedErr:               true,
		},
		"nodebucket doesn't exist": {
			enableNodePool:            true,
			enablePoolServiceTopology: true,
			poolName:                  "hangzhou",
			yurtClient:                fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind),
			expectedErr:               true,
		},
		"disable pool service topology feature": {
			enableNodePool:            false,
			enablePoolServiceTopology: false,
			poolName:                  "hangzhou",
			yurtClient:                fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nodeBucketGVRToListKind),
			expectedErr:               false,
			expectedNodes:             sets.NewString(),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			yurtFactory := dynamicinformer.NewDynamicSharedInformerFactory(tc.yurtClient, 24*time.Hour)
			initializer := NewNodesInitializer(tc.enableNodePool, tc.enablePoolServiceTopology, yurtFactory)

			stopper := make(chan struct{})
			defer close(stopper)
			yurtFactory.Start(stopper)
			yurtFactory.WaitForCacheSync(stopper)

			nopFilter := &nopNodeHandler{}
			if err := initializer.Initialize(nopFilter); err != nil {
				t.Errorf("couldn't initialize filter, %v", err)
				return
			}

			if !nopFilter.nodesSynced() {
				t.Errorf("nodes is not synced")
				return
			}

			nodes, err := nopFilter.nodesGetter(tc.poolName)
			if tc.expectedErr && err == nil {
				t.Errorf("expect error, but got nil")
				return
			} else if !tc.expectedErr && err != nil {
				t.Errorf("couldn't get nodes, %v", err)
				return
			} else if tc.expectedErr {
				return
			}

			if !tc.expectedNodes.Equal(sets.NewString(nodes...)) {
				t.Errorf("expect nodes %v, but got %v", tc.expectedNodes, nodes)
			}
		})
	}
}
