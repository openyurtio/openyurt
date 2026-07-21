/*
Copyright 2022 The OpenYurt Authors.

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

package init

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func newRuntimeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := appsv1beta2.AddToScheme(s); err != nil {
		t.Fatalf("failed to add apps scheme: %v", err)
	}

	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestClusterConverter_CreateDefaultNodePools(t *testing.T) {
	converter := &ClusterConverter{
		RuntimeClient: newRuntimeClient(t),
	}

	if err := converter.createDefaultNodePools(); err != nil {
		t.Fatalf("createDefaultNodePools returned error: %v", err)
	}

	for name := range DefaultPools {
		pool := &appsv1beta2.NodePool{}
		if err := converter.RuntimeClient.Get(context.Background(), client.ObjectKey{Name: name}, pool); err != nil {
			t.Fatalf("failed to get nodepool %s: %v", name, err)
		}
	}
}

func TestClusterConverter_AssignNodesToNodePools(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "openyurt-e2e-test-worker",
			Labels: map[string]string{},
		},
	}
	converter := &ClusterConverter{
		RuntimeClient: newRuntimeClient(t, node),
		EdgeNodes:     []string{node.Name},
	}

	targetNodes, err := converter.assignNodesToNodePools()
	if err != nil {
		t.Fatalf("assignNodesToNodePools returned error: %v", err)
	}
	if len(targetNodes) != 1 || targetNodes[0] != node.Name {
		t.Fatalf("unexpected target nodes: %v", targetNodes)
	}

	updatedNode := &corev1.Node{}
	if err := converter.RuntimeClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("failed to get patched node: %v", err)
	}
	if updatedNode.Labels[projectinfo.GetNodePoolLabel()] != NodeNameToPool[node.Name] {
		t.Fatalf("unexpected nodepool label: %v", updatedNode.Labels)
	}
}

func TestClusterConverter_WaitNodesConverted(t *testing.T) {
	nodeName := "openyurt-e2e-test-worker"
	expectedPool := NodeNameToPool[nodeName]

	newNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					projectinfo.GetNodePoolLabel():      expectedPool,
					projectinfo.GetEdgeWorkerLabelKey(): "true",
				},
			},
		}
	}

	newJob := func(jobType batchv1.JobConditionType) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-servant-conversion-test",
				Namespace: nodeservant.DefaultConversionJobNamespace,
				Labels: map[string]string{
					nodeservant.ConversionNodeLabelKey: nodeName,
				},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{{
					Type:   jobType,
					Status: corev1.ConditionTrue,
				}},
			},
		}
	}

	tests := map[string]struct {
		objects []runtime.Object
		wantErr string
	}{
		"settled when node labels and jobs are complete": {
			objects: []runtime.Object{
				newNode(),
				newJob(batchv1.JobComplete),
			},
		},
		"failed conversion condition stops the wait": {
			objects: []runtime.Object{
				func() *corev1.Node {
					node := newNode()
					node.Status.Conditions = []corev1.NodeCondition{{
						Type:    conversionConditionType,
						Status:  corev1.ConditionTrue,
						Reason:  "ConvertFailed",
						Message: "boom",
					}}
					return node
				}(),
			},
			wantErr: "conversion failed",
		},
		"failed conversion job stops the wait": {
			objects: []runtime.Object{
				newNode(),
				newJob(batchv1.JobFailed),
			},
			wantErr: "failed",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			converter := &ClusterConverter{
				ClientSet:                 clientsetfake.NewSimpleClientset(tt.objects...),
				WaitNodeConversionTimeout: time.Second,
				EdgeNodes:                 []string{nodeName},
			}

			err := converter.waitNodesConverted([]string{nodeName})
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("waitNodesConverted returned error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestIsNodeConversionSettled(t *testing.T) {
	tests := map[string]struct {
		node         *corev1.Node
		jobs         []batchv1.Job
		expectedPool string
		wantSettled  bool
		wantErr      bool
	}{
		"stable node with no active job": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel():      "pool-a",
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			expectedPool: "pool-a",
			wantSettled:  true,
		},
		"node missing edge-worker label": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "pool-a",
					},
				},
			},
			expectedPool: "pool-a",
		},
		"running conversion job": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel():      "pool-a",
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			jobs: []batchv1.Job{{
				ObjectMeta: metav1.ObjectMeta{Name: "job-a"},
			}},
			expectedPool: "pool-a",
		},
		"failed condition stops wait": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel():      "pool-a",
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{{
						Type:    conversionConditionType,
						Status:  corev1.ConditionTrue,
						Reason:  "ConvertFailed",
						Message: "boom",
					}},
				},
			},
			expectedPool: "pool-a",
			wantErr:      true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			settled, err := isNodeConversionSettled(tt.node, tt.expectedPool, tt.jobs)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if settled != tt.wantSettled {
				t.Fatalf("unexpected settled state: got %t want %t", settled, tt.wantSettled)
			}
		})
	}
}

func TestImageTag(t *testing.T) {
	tag, err := imageTag("openyurt/node-servant:v1.6.1")
	if err != nil {
		t.Fatalf("imageTag returned error: %v", err)
	}
	if tag != "v1.6.1" {
		t.Fatalf("unexpected tag %q", tag)
	}

	if _, err := imageTag("openyurt/node-servant"); err == nil {
		t.Fatalf("expected error for image without tag")
	}
}
