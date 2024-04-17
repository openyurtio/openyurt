/*
Copyright 2024 The OpenYurt Authors.

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

package autonomy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestFormat(t *testing.T) {
	assert.Equal(t, "autonomy-controller: autonomy-controller add controller Node", Format("autonomy-controller add controller %s", "Node"))
}

func TestReconcileAutonomy_Reconcile(t *testing.T) {
	testcases := []struct {
		name string
		node corev1.Node
	}{
		{
			name: "test1",
			node: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
					Annotations: map[string]string{
						projectinfo.GetAutonomyAnnotation(): "true",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   appsv1beta1.NodeAutonomy,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fakeclient.NewClientBuilder().WithObjects(&tc.node).Build()
			r := &ReconcileAutonomy{
				Client: fakeClient,
			}
			_, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: tc.node.Name,
				},
			})
			assert.Equal(t, nil, err)
			var node corev1.Node
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name: tc.node.Name,
			}, &node)
			assert.Equal(t, nil, err)
			assert.Contains(t, node.Labels, projectinfo.GetAutonomyStatusLabel())
			assert.Equal(t, "true", node.Labels[projectinfo.GetAutonomyStatusLabel()])
		})
	}
}

func TestPrepare(t *testing.T) {
	testcases := []struct {
		name         string
		eventType    string
		createEvent  event.CreateEvent
		updateEvent  event.UpdateEvent
		deleteEvent  event.DeleteEvent
		genericEvent event.GenericEvent
		want         bool
	}{
		{
			name:      "test1",
			eventType: "create",
			createEvent: event.CreateEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
						Labels: map[string]string{
							projectinfo.GetEdgeWorkerLabelKey(): "true",
						},
						Annotations: map[string]string{
							projectinfo.GetAutonomyAnnotation(): "true",
						},
					},
				},
			},
			want: true,
		},
		{
			name:      "test2",
			eventType: "create",
			createEvent: event.CreateEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
						Labels: map[string]string{
							projectinfo.GetEdgeWorkerLabelKey(): "false",
						},
						Annotations: map[string]string{
							projectinfo.GetAutonomyAnnotation(): "true",
						},
					},
				},
			},
			want: false,
		},
		{
			name:      "test3",
			eventType: "update",
			updateEvent: event.UpdateEvent{
				ObjectOld: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
						Annotations: map[string]string{
							projectinfo.GetAutonomyAnnotation(): "true",
						},
					},
				},
				ObjectNew: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
						Annotations: map[string]string{
							projectinfo.GetAutonomyAnnotation(): "true",
						},
					},
				},
			},
			want: false,
		},
		{
			name:         "test4",
			eventType:    "generic",
			genericEvent: event.GenericEvent{},
			want:         false,
		},
		{
			name:      "test5",
			eventType: "delete",
			want:      false,
		},
	}
	nodePredicate := prepare()
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.eventType {
			case "create":
				if nodePredicate.Create(tc.createEvent) != tc.want {
					t.Errorf("create event should be %v", tc.want)
				}
			case "delete":
				if nodePredicate.Delete(tc.deleteEvent) != tc.want {
					t.Errorf("delete event should be %v", tc.want)
				}
			case "update":
				if nodePredicate.Update(tc.updateEvent) != tc.want {
					t.Errorf("update event should be %v", tc.want)
				}
			case "generic":
				if nodePredicate.Generic(tc.genericEvent) != tc.want {
					t.Errorf("generic event should be %v", tc.want)
				}
			}
		})
	}
}
