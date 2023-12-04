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

package nodebucket

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsalphav1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal("Fail to add kubernetes clint-go custom resource")
	}
	apis.AddToScheme(scheme)

	testcases := map[string]struct {
		maxNodesPerBucket     int32
		nodes                 []client.Object
		nodeBuckets           []client.Object
		pool                  client.Object
		wantedNumberOfBuckets int
		wantedNodeNames       sets.String
	}{
		"generate one bucket": {
			maxNodesPerBucket: 10,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 1,
			wantedNodeNames:       sets.NewString("node1", "node2"),
		},
		"generate two buckets": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 2,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node5"),
		},
		"update one bucket": {
			maxNodesPerBucket: 10,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-xxxxxx",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node6",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 1,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node5"),
		},
		"update two buckets": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-bar",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node6",
						},
					},
				},
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-foo",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node7",
						},
						{
							Name: "node8",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 2,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node5"),
		},
		"create and update one bucket": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "shanghai",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-bar",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node6",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 2,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node5"),
		},
		"delete and update one bucket": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-bar",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
					},
				},
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-foo",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node7",
						},
						{
							Name: "node8",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 1,
			wantedNodeNames:       sets.NewString("node1", "node2"),
		},
		"update three buckets": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-bar",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node6",
						},
					},
				},
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-foo",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node2",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 3,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node4", "node5"),
		},
		"two buckets are updated": {
			maxNodesPerBucket: 2,
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
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
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node4",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node5",
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "hangzhou",
						},
					},
				},
			},
			nodeBuckets: []client.Object{
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-bar",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node1",
						},
						{
							Name: "node6",
						},
					},
				},
				&appsalphav1.NodeBucket{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hangzhou-foo",
						Labels: map[string]string{
							LabelNodePoolName: "hangzhou",
						},
					},
					Nodes: []appsalphav1.Node{
						{
							Name: "node2",
						},
						{
							Name: "node7",
						},
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou",
				},
			},
			wantedNumberOfBuckets: 3,
			wantedNodeNames:       sets.NewString("node1", "node2", "node3", "node4", "node5"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cb := fakeclient.NewClientBuilder().WithScheme(scheme)
			if len(tc.nodes) != 0 {
				cb = cb.WithObjects(tc.nodes...)
			}

			if len(tc.nodeBuckets) != 0 {
				cb = cb.WithObjects(tc.nodeBuckets...)
			}

			if tc.pool != nil {
				cb = cb.WithObjects([]client.Object{tc.pool}...)
			}
			c := cb.Build()
			r := &ReconcileNodeBucket{
				Client:            c,
				maxNodesPerBucket: int(tc.maxNodesPerBucket),
			}

			if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: tc.pool.GetName()}}); err != nil {
				t.Errorf("could not reconcile pool, %v", err)
			}

			buckets := new(appsalphav1.NodeBucketList)
			if err := c.List(context.Background(), buckets, &client.MatchingLabels{LabelNodePoolName: tc.pool.GetName()}); err != nil {
				t.Errorf("could not list node buckets, %v", err)
			}

			if len(buckets.Items) != tc.wantedNumberOfBuckets {
				t.Errorf("expect %d buckets, but got %d", tc.wantedNumberOfBuckets, len(buckets.Items))
			}

			gotBucketNodes := sets.String{}
			for i := range buckets.Items {
				for _, node := range buckets.Items[i].Nodes {
					gotBucketNodes.Insert(node.Name)
				}
			}

			if !tc.wantedNodeNames.Equal(gotBucketNodes) {
				t.Errorf("expect nodes %v, but got %v", tc.wantedNodeNames.UnsortedList(), gotBucketNodes.UnsortedList())
			}
		})
	}
}
