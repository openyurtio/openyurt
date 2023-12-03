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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestCreate(t *testing.T) {
	testcases := map[string]struct {
		event     event.CreateEvent
		wantedNum int
	}{
		"add a pod": {
			event: event.CreateEvent{
				Object: &corev1.Pod{},
			},
			wantedNum: 0,
		},
		"node doesn't belong to a pool": {
			event: event.CreateEvent{
				Object: &corev1.Node{},
			},
			wantedNum: 0,
		},
		"node belongs to a pool": {
			event: event.CreateEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
			},
			wantedNum: 1,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			handler := &EnqueueNodePoolForNode{}
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			handler.Create(tc.event, q)

			if q.Len() != tc.wantedNum {
				t.Errorf("Expected %d, got %d", tc.wantedNum, q.Len())
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	testcases := map[string]struct {
		event     event.UpdateEvent
		wantedNum int
	}{
		"invalid old object": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Pod{},
				ObjectNew: &corev1.Node{},
			},
			wantedNum: 0,
		},
		"invalid new object": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{},
				ObjectNew: &corev1.Pod{},
			},
			wantedNum: 0,
		},
		"update orphan node": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{},
				ObjectNew: &corev1.Node{},
			},
			wantedNum: 0,
		},
		"add a node into pool": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
			},
			wantedNum: 1,
		},
		"pool of node is changed": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "bar",
						},
					},
				},
			},
			wantedNum: 0,
		},
		"pool of node is not changed": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
			},
			wantedNum: 0,
		},
		"node ready status is changed": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
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
			wantedNum: 1,
		},
		"node ready status is not changed": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
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
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
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
			wantedNum: 0,
		},
		"node labels is changed": {
			event: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
							"label1":                       "value1",
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
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
							"label2":                       "value2",
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
			wantedNum: 1,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			handler := &EnqueueNodePoolForNode{
				EnableSyncNodePoolConfigurations: true,
				Recorder:                         record.NewFakeRecorder(100),
			}
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			handler.Update(tc.event, q)

			if q.Len() != tc.wantedNum {
				t.Errorf("Expected %d, got %d", tc.wantedNum, q.Len())
			}
		})
	}
}

func TestDelete(t *testing.T) {
	testcases := map[string]struct {
		event     event.DeleteEvent
		wantedNum int
	}{
		"delete a pod": {
			event: event.DeleteEvent{
				Object: &corev1.Pod{},
			},
			wantedNum: 0,
		},
		"delete a orphan node": {
			event: event.DeleteEvent{
				Object: &corev1.Node{},
			},
			wantedNum: 0,
		},
		"delete a pool node": {
			event: event.DeleteEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							projectinfo.GetNodePoolLabel(): "foo",
						},
					},
				},
			},
			wantedNum: 1,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			handler := &EnqueueNodePoolForNode{}
			q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			handler.Delete(tc.event, q)

			if q.Len() != tc.wantedNum {
				t.Errorf("Expected %d, got %d", tc.wantedNum, q.Len())
			}
		})
	}
}
