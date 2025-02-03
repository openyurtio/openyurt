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

package gatewayinternalservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

func TestEnqueueRequestForGatewayEvent(t *testing.T) {
	h := &EnqueueRequestForGatewayEvent{}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	ctx := context.Background()

	tests := []struct {
		name         string
		expectedLen  int
		eventHandler func()
	}{
		{
			name:        "should get work queue len is 0 when Create Object is not Gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Create Gateway not have ExposeType",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: &ravenv1beta1.Gateway{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create Gateway",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: mockGateway()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete Object is not Gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete Gateway not have ExposeType",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: &ravenv1beta1.Gateway{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Delete Gateway",
			expectedLen: 1,
			eventHandler: func() {
				h.Delete(ctx, event.DeleteEvent{Object: mockGateway()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update old object is not Gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: &unstructured.Unstructured{}, ObjectNew: mockGateway()}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update new object is not Gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: mockGateway(), ObjectNew: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update Gateway not have ExposeType",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(ctx, event.UpdateEvent{ObjectOld: &ravenv1beta1.Gateway{}, ObjectNew: &ravenv1beta1.Gateway{}}, queue)
			},
		},
		{
			name: "should get work queue len is 1 when Update Gateway success",
			eventHandler: func() {
				oldGateWay := mockGateway()
				newGateWay := oldGateWay.DeepCopy()
				newGateWay.ObjectMeta.Name = "new" + MockGateway
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldGateWay, ObjectNew: newGateWay}, queue)
			},
			expectedLen: 1,
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

func TestEnqueueRequestForConfigEvent(t *testing.T) {
	h := &EnqueueRequestForConfigEvent{}
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
			name:        "should get work queue len is 1 when Create ConfigMap with invalid ProxyServerInsecurePortKey",
			expectedLen: 1,
			eventHandler: func() {
				configMap := mockConfigMap()
				configMap.Data[util.ProxyServerInsecurePortKey] = "127.0.0.1"
				h.Create(ctx, event.CreateEvent{Object: configMap}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create ConfigMap with valid ProxyServerInsecurePortKey",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(ctx, event.CreateEvent{Object: mockConfigMap()}, queue)
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
			name: "should get work queue len is 1 when update ConfigMap with new InsecurePortKey",
			eventHandler: func() {
				oldConfigMap := mockConfigMap()
				newConfigMap := oldConfigMap.DeepCopy()
				newConfigMap.Data[util.ProxyServerInsecurePortKey] = "127.0.0.1:90"
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}, queue)
			},
			expectedLen: 1,
		},
		{
			name: "should get work queue len is 1 when Update ConfigMap with new SecurePortKey",
			eventHandler: func() {
				oldConfigMap := mockConfigMap()
				newConfigMap := oldConfigMap.DeepCopy()
				newConfigMap.Data[util.ProxyServerSecurePortKey] = "127.0.0.2:90"
				h.Update(ctx, event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}, queue)
			},
			expectedLen: 1,
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

func mockGateway() *ravenv1beta1.Gateway {
	return &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: MockGateway,
		},
		Spec: ravenv1beta1.GatewaySpec{
			ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
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
			util.RavenEnableProxy:           "true",
			util.RavenEnableTunnel:          "true",
			util.ProxyServerInsecurePortKey: "127.0.0.1:80",
			util.ProxyServerSecurePortKey:   "127.0.0.2:80",
		},
	}
}

func clearQueue(queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	for queue.Len() > 0 {
		item, _ := queue.Get()
		queue.Done(item)
	}
}
