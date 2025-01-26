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

package gatewaypickup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

// TestEnqueueGatewayForNode tests the method of EnqueueGatewayForNode.
func TestEnqueueGatewayForNode(t *testing.T) {
	h := &EnqueueGatewayForNode{}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	ctx := context.Background()

	tests := []struct {
		name         string
		expectedLen  int
		eventHandler func()
	}{
		{
			name:        "should get work queue len is 0 when Create Object is not node",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Create Node not have LabelCurrentGateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: &corev1.Node{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create Node",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: mockNode()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete Object is not node",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete Node not have LabelCurrentGateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: &corev1.Node{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Delete Node",
			expectedLen: 1,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: mockNode()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update old object is not node",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: &unstructured.Unstructured{}, ObjectNew: mockNode()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update new object is not node",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: mockNode(), ObjectNew: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update Node with new gateway label",
			expectedLen: 2,
			eventHandler: func() {
				oldNode := mockNode()
				newNode := oldNode.DeepCopy()
				newNode.ObjectMeta.Labels[raven.LabelCurrentGateway] = "gw-mock-new"
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 Update Node with status change",
			expectedLen: 1,
			eventHandler: func() {
				oldNode := mockNode()
				newNode := oldNode.DeepCopy()
				newNode.Status.Conditions[0].Status = corev1.ConditionFalse
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, queue)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.eventHandler()
			assert.Equal(t, tc.expectedLen, queue.Len(), "Unexpected queue length in test: %s", tc.name)
			clearQueue(queue)
		})
	}
}

// TestEnqueueGatewayForRavenConfig tests the method of EnqueueGatewayForRavenConfig.
func TestEnqueueGatewayForRavenConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = ravenv1beta1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	h := EnqueueGatewayForRavenConfig{
		client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(mockObjs()...).Build(),
	}

	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	ctx := context.Background()

	tests := []struct {
		name         string
		expectedLen  int
		eventHandler func()
	}{
		{
			name:        "should get work queue len is 0 when Create Object is not ConfigMap",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create ConfigMap",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: mockConfigMap()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete Object is not ConfigMap",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Delete ConfigMap",
			expectedLen: 1,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: mockConfigMap()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update old object is not ConfigMap",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: &unstructured.Unstructured{}, ObjectNew: mockConfigMap()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update new object is not ConfigMap",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: mockConfigMap(), ObjectNew: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update ConfigMap with RavenEnableProxy change",
			expectedLen: 1,
			eventHandler: func() {
				oldConfigMap := mockConfigMap()
				newConfigMap := oldConfigMap.DeepCopy()
				newConfigMap.Data[util.RavenEnableProxy] = "false"
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update ConfigMap with RavenEnableTunnel change",
			expectedLen: 1,
			eventHandler: func() {
				oldConfigMap := mockConfigMap()
				newConfigMap := oldConfigMap.DeepCopy()
				newConfigMap.Data[util.RavenEnableTunnel] = "false"
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}, queue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.eventHandler()
			assert.Equal(t, tt.expectedLen, queue.Len(), "Unexpected queue length in test: %s", tt.name)
			clearQueue(queue)
		})
	}
}

func mockObjs() []runtime.Object {
	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
		},
	}
	configmaps := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "default",
				},
				Data: map[string]string{
					util.RavenEnableProxy:  "true",
					util.RavenEnableTunnel: "true",
				},
			},
		},
	}
	gateways := &ravenv1beta1.GatewayList{
		Items: []ravenv1beta1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gw-mock",
				},
			},
		},
	}

	return []runtime.Object{nodeList, gateways, configmaps}
}

func mockNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node",
			Labels: map[string]string{
				raven.LabelCurrentGateway: "gw-mock",
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
	}
}

func mockConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "default",
		},
		Data: map[string]string{
			util.RavenEnableProxy:  "true",
			util.RavenEnableTunnel: "true",
		},
	}
}

func clearQueue(queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	for queue.Len() > 0 {
		item, _ := queue.Get()
		queue.Done(item)
	}
}
