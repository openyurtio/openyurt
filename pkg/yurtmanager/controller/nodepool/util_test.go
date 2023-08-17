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
	"encoding/json"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func TestConcilateNode(t *testing.T) {
	testcases := map[string]struct {
		initNpra                   *NodePoolRelatedAttributes
		mockNode                   corev1.Node
		pool                       appsv1beta1.NodePool
		wantedNodeExcludeAttribute corev1.Node
		updated                    bool
	}{
		"node has no pool attributes": {
			mockNode: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			pool: appsv1beta1.NodePool{
				Spec: appsv1beta1.NodePoolSpec{
					Labels: map[string]string{
						"poollabel1": "value1",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"poolanno1": "value1",
						"poolanno2": "value2",
					},
					Taints: []corev1.Taint{
						{
							Key:    "poolkey1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantedNodeExcludeAttribute: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1":     "value1",
						"label2":     "value2",
						"poollabel1": "value1",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"anno1":     "value1",
						"poolanno1": "value1",
						"poolanno2": "value2",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "poolkey1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			updated: true,
		},
		"node has some pool attributes": {
			initNpra: &NodePoolRelatedAttributes{
				Labels: map[string]string{
					"label2": "value2",
				},
				Annotations: map[string]string{
					"anno2": "value2",
					"anno3": "value3",
				},
				Taints: []corev1.Taint{
					{
						Key:    "key1",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			mockNode: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
						"anno2": "value2",
						"anno3": "value3",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key2",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key3",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			pool: appsv1beta1.NodePool{
				Spec: appsv1beta1.NodePoolSpec{
					Labels: map[string]string{
						"poollabel1": "value1",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"poolanno1": "value1",
						"poolanno2": "value2",
					},
					Taints: []corev1.Taint{
						{
							Key:    "poolkey1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantedNodeExcludeAttribute: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1":     "value1",
						"poollabel1": "value1",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"anno1":     "value1",
						"poolanno1": "value1",
						"poolanno2": "value2",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key2",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key3",
							Effect: corev1.TaintEffectNoExecute,
						},
						{
							Key:    "poolkey1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			updated: true,
		},
		"pool attributes is not changed": {
			initNpra: &NodePoolRelatedAttributes{
				Labels: map[string]string{
					"label2": "value2",
				},
				Annotations: map[string]string{
					"anno2": "value2",
					"anno3": "value3",
				},
				Taints: []corev1.Taint{
					{
						Key:    "key1",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			mockNode: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
						"anno2": "value2",
						"anno3": "value3",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key2",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key3",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			pool: appsv1beta1.NodePool{
				Spec: appsv1beta1.NodePoolSpec{
					Labels: map[string]string{
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno2": "value2",
						"anno3": "value3",
					},
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantedNodeExcludeAttribute: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
						"anno2": "value2",
						"anno3": "value3",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key2",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "key3",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
			updated: false,
		},
		"pool has some duplicated attributes": {
			mockNode: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
						"anno2": "value2",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			pool: appsv1beta1.NodePool{
				Spec: appsv1beta1.NodePoolSpec{
					Labels: map[string]string{
						"label2":     "value2",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
					},
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			wantedNodeExcludeAttribute: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1":     "value1",
						"label2":     "value2",
						"poollabel2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
						"anno2": "value2",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			updated: true,
		},
		"node and pool has no pool attributes": {
			mockNode: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			pool: appsv1beta1.NodePool{
				Spec: appsv1beta1.NodePoolSpec{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
					Taints:      []corev1.Taint{},
				},
			},
			wantedNodeExcludeAttribute: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label1": "value1",
						"label2": "value2",
					},
					Annotations: map[string]string{
						"anno1": "value1",
					},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{
							Key:    "key1",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			updated: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			if tc.initNpra != nil {
				if err := encodePoolAttrs(&tc.mockNode, tc.initNpra); err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}

			changed, err := conciliateNode(&tc.mockNode, &tc.pool)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}

			if tc.updated != changed {
				t.Errorf("Expected %v, got %v, node %#+v", tc.updated, changed, tc.mockNode)
			}

			wantedNpra := NodePoolRelatedAttributes{
				Labels:      tc.pool.Spec.Labels,
				Annotations: tc.pool.Spec.Annotations,
				Taints:      tc.pool.Spec.Taints,
			}
			npra, err := decodePoolAttrs(&tc.mockNode)
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if !areNodePoolRelatedAttributesEqual(npra, &wantedNpra) {
				t.Errorf("Expected %v, got %v", wantedNpra, *npra)
			}

			delete(tc.mockNode.Annotations, apps.AnnotationPrevAttrs)
			if !reflect.DeepEqual(tc.mockNode, tc.wantedNodeExcludeAttribute) {
				t.Errorf("Expected %v, got %v", tc.wantedNodeExcludeAttribute, tc.mockNode)
			}
		})
	}
}

func TestConciliateLabels(t *testing.T) {
	mockNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
	}

	oldLabels := map[string]string{
		"label1": "value1",
	}
	newLabels := map[string]string{
		"label3": "value3",
		"label4": "value4",
	}

	wantNodeLabels := map[string]string{
		"label2": "value2",
		"label3": "value3",
		"label4": "value4",
	}

	conciliateLabels(mockNode, oldLabels, newLabels)
	if !reflect.DeepEqual(wantNodeLabels, mockNode.Labels) {
		t.Errorf("Expected %v, got %v", wantNodeLabels, mockNode.Labels)
	}
}

func TestConciliateAnnotations(t *testing.T) {
	mockNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"anno1": "value1",
				"anno2": "value2",
			},
		},
	}

	oldAnnos := map[string]string{
		"anno1": "value1",
	}
	newAnnos := map[string]string{
		"anno3": "value3",
		"anno4": "value4",
	}

	wantNodeAnnos := map[string]string{
		"anno2": "value2",
		"anno3": "value3",
		"anno4": "value4",
	}

	conciliateAnnotations(mockNode, oldAnnos, newAnnos)
	if !reflect.DeepEqual(wantNodeAnnos, mockNode.Annotations) {
		t.Errorf("Expected %v, got %v", wantNodeAnnos, mockNode.Annotations)
	}
}

func TestConciliateTaints(t *testing.T) {
	mockNode := &corev1.Node{
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    "key1",
					Value:  "value1",
					Effect: corev1.TaintEffectNoExecute,
				},
				{
					Key:    "key2",
					Value:  "value2",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
	}

	// Test case where oldTaint is present in the node and should be removed
	oldTaints := []corev1.Taint{
		{
			Key:    "key1",
			Value:  "value1",
			Effect: corev1.TaintEffectNoExecute,
		},
	}
	newTaints := []corev1.Taint{
		{
			Key:    "key3",
			Value:  "value3",
			Effect: corev1.TaintEffectPreferNoSchedule,
		},
	}
	wantNodeTaints := []corev1.Taint{
		{
			Key:    "key2",
			Value:  "value2",
			Effect: corev1.TaintEffectNoSchedule,
		},
		{
			Key:    "key3",
			Value:  "value3",
			Effect: corev1.TaintEffectPreferNoSchedule,
		},
	}
	conciliateTaints(mockNode, oldTaints, newTaints)

	if !reflect.DeepEqual(wantNodeTaints, mockNode.Spec.Taints) {
		t.Errorf("Expected %v, got %v", wantNodeTaints, mockNode.Spec.Taints)
	}
}

func TestConciliateNodePoolStatus(t *testing.T) {
	testcases := map[string]struct {
		readyNodes    int32
		notReadyNodes int32
		nodes         []string
		pool          *appsv1beta1.NodePool
		needUpdated   bool
	}{
		"status is needed to update": {
			readyNodes:    5,
			notReadyNodes: 2,
			nodes:         []string{"foo", "bar", "cat", "zxxde"},
			pool: &appsv1beta1.NodePool{
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   2,
					UnreadyNodeNum: 3,
					Nodes:          []string{"foo", "bar", "cat", "zxxde", "lucky"},
				},
			},
			needUpdated: true,
		},
		"status is not updated": {
			readyNodes:    2,
			notReadyNodes: 2,
			nodes:         []string{"foo", "bar", "cat", "zxxde"},
			pool: &appsv1beta1.NodePool{
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   2,
					UnreadyNodeNum: 2,
					Nodes:          []string{"foo", "bar", "cat", "zxxde"},
				},
			},
			needUpdated: false,
		},
		"status is not updated when pool is empty": {
			readyNodes:    0,
			notReadyNodes: 0,
			nodes:         []string{},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: appsv1beta1.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
			needUpdated: false,
		},
		"status is not updated when pool has no status": {
			readyNodes:    0,
			notReadyNodes: 0,
			nodes:         []string{},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			needUpdated: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			needUpdated := conciliateNodePoolStatus(tc.readyNodes, tc.notReadyNodes, tc.nodes, tc.pool)
			if needUpdated != tc.needUpdated {
				t.Errorf("Expected %v, got %v", tc.needUpdated, needUpdated)
			}
		})
	}
}

func TestContainTaint(t *testing.T) {
	mockTaints := []corev1.Taint{
		{
			Key:    "key1",
			Value:  "value1",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "key2",
			Value:  "value2",
			Effect: corev1.TaintEffectNoSchedule,
		},
	}
	testcases := map[string]struct {
		inputTaint  corev1.Taint
		resultIndex int
		isContained bool
	}{
		"taint is contained": {
			inputTaint: corev1.Taint{
				Key:    "key1",
				Value:  "value1",
				Effect: corev1.TaintEffectNoExecute,
			},
			resultIndex: 0,
			isContained: true,
		},
		"taint is not contained": {
			inputTaint: corev1.Taint{
				Key:    "key3",
				Value:  "value3",
				Effect: corev1.TaintEffectPreferNoSchedule,
			},
			resultIndex: 0,
			isContained: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			gotIndex, gotBool := containTaint(tc.inputTaint, mockTaints)
			if gotIndex != tc.resultIndex || gotBool != tc.isContained {
				t.Errorf("Expected index %v and bool %v, got index %v and bool %v", tc.resultIndex, tc.isContained, gotIndex, gotBool)
			}
		})
	}
}

func TestRemoveTaint(t *testing.T) {
	mockTaints := []corev1.Taint{
		{
			Key:    "key1",
			Value:  "value1",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "key2",
			Value:  "value2",
			Effect: corev1.TaintEffectNoSchedule,
		},
	}

	testcases := map[string]struct {
		mockTaint  corev1.Taint
		wantTaints []corev1.Taint
	}{
		"remove exist taint": {
			mockTaint: corev1.Taint{
				Key:    "key1",
				Value:  "value1",
				Effect: corev1.TaintEffectNoExecute,
			},
			wantTaints: []corev1.Taint{
				{
					Key:    "key2",
					Value:  "value2",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		"remove not exist taint": {
			mockTaint: corev1.Taint{
				Key:    "key3",
				Value:  "value3",
				Effect: corev1.TaintEffectPreferNoSchedule,
			},
			wantTaints: mockTaints,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			gotTaints := removeTaint(tc.mockTaint, mockTaints)
			if !reflect.DeepEqual(tc.wantTaints, gotTaints) {
				t.Errorf("Expected %v, got %v", tc.wantTaints, gotTaints)
			}
		})
	}
}

func TestEncodePoolAttrs(t *testing.T) {
	testcases := map[string]struct {
		mockNode *corev1.Node
		mockNpra *NodePoolRelatedAttributes
	}{
		"annotations is not set": {
			mockNode: &corev1.Node{},
			mockNpra: &NodePoolRelatedAttributes{
				Labels: map[string]string{
					"foo": "bar",
				},
			},
		},
		"annotations is set": {
			mockNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			mockNpra: &NodePoolRelatedAttributes{
				Labels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			gotErr := encodePoolAttrs(tc.mockNode, tc.mockNpra)
			if gotErr != nil {
				t.Errorf("Expected no error, got %v", gotErr)
			}

			// Ensure that the NodePoolRelatedAttributes has been correctly stored in the node's annotations
			gotNpra, err := decodePoolAttrs(tc.mockNode)
			if err != nil || !reflect.DeepEqual(gotNpra, tc.mockNpra) {
				t.Errorf("Expected %v, got %v", tc.mockNpra, gotNpra)
			}
		})
	}
}

func TestDecodePoolAttrs(t *testing.T) {
	wantNpra := &NodePoolRelatedAttributes{
		Labels: map[string]string{
			"foo": "bar",
		},
	}
	npraJson, err := json.Marshal(wantNpra)
	if err != nil {
		t.Errorf("failed to marshal npra")
	}

	mockNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				apps.AnnotationPrevAttrs: string(npraJson),
			},
		},
	}
	gotNpra, gotErr := decodePoolAttrs(mockNode)

	if gotErr != nil {
		t.Errorf("Expected no error, got %v", gotErr)
	}

	if !reflect.DeepEqual(wantNpra, gotNpra) {
		t.Errorf("Expected %v, got %v", wantNpra, gotNpra)
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name string
		node corev1.Node
		want bool
	}{
		{
			name: "NodeReady and ConditionTrue",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "NodeReady but ConditionFalse",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Node status not NodeReady",
			node: corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNodeReady(tt.node); got != tt.want {
				t.Errorf("isNodeReady() = %v, want %v", got, tt.want)
			}
		})
	}
}
