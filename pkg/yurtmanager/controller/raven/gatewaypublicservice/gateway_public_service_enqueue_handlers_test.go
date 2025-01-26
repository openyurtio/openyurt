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

package gatewaypublicservice

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

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

// TestEnqueueRequestForGatewayEvent tests the method of EnqueueRequestForGatewayEvent.
func TestEnqueueRequestForGatewayEvent(t *testing.T) {
	h := &EnqueueRequestForGatewayEvent{}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	deletionTime := metav1.Now()

	tests := []struct {
		name         string
		expectedLen  int
		eventHandler func()
	}{
		{
			name:        "should get work queue len is 0 when create event type is not gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Create Public IP Gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: mockGateway(MockGateway, ravenv1beta1.ExposeTypePublicIP)}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create LoadBalancer Gateway",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: mockGateway(MockGateway, ravenv1beta1.ExposeTypeLoadBalancer)}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when delete event type is not gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(context.Background(), event.DeleteEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Delete gateway ExposeType not LoadBalancer",
			expectedLen: 0,
			eventHandler: func() {
				h.Delete(context.Background(), event.DeleteEvent{
					Object: func() *ravenv1beta1.Gateway {
						gw := mockGateway(MockGateway, ravenv1beta1.ExposeTypePublicIP)
						gw.DeletionTimestamp = &deletionTime
						return gw
					}(),
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Delete gateway ExposeType LoadBalancer",
			expectedLen: 1,
			eventHandler: func() {
				h.Delete(context.Background(), event.DeleteEvent{
					Object: func() *ravenv1beta1.Gateway {
						gw := mockGateway(MockGateway, ravenv1beta1.ExposeTypeLoadBalancer)
						gw.DeletionTimestamp = &deletionTime
						return gw
					}(),
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when update old type is not gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: &unstructured.Unstructured{},
					ObjectNew: mockGateway(MockGateway, ravenv1beta1.ExposeTypeLoadBalancer),
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when update event type is not gateway",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: mockGateway(MockGateway, ravenv1beta1.ExposeTypeLoadBalancer),
					ObjectNew: &unstructured.Unstructured{},
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update Gateway Label",
			expectedLen: 1,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: mockGateway(MockGateway, ravenv1beta1.ExposeTypePublicIP),
					ObjectNew: mockGateway(MockGateway, ravenv1beta1.ExposeTypeLoadBalancer),
				}, queue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.eventHandler()
			assert.Equal(t, tt.expectedLen, queue.Len(), "Unexpected queue length")
			clearQueue(queue)
		})
	}
}

// TestEnqueueRequestForConfigEvent tests the method of EnqueueRequestForConfigEvent.
func TestEnqueueRequestForConfigEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = ravenv1beta1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)

	h := EnqueueRequestForConfigEvent{client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(mockObjs()...).Build()}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	tests := []struct {
		name         string
		expectedLen  int
		eventHandler func()
	}{
		{
			name:        "should get work queue len is 0 when Create event type is ConfigMap",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: &unstructured.Unstructured{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Create ConfigMap data is empty",
			expectedLen: 0,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: &corev1.ConfigMap{}}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create ConfigMap data is Proxy",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: mockConfigMap(util.ProxyServerExposedPortKey, "1.1.1.3:80")}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Create ConfigMap data is VPN",
			expectedLen: 1,
			eventHandler: func() {
				h.Create(context.Background(), event.CreateEvent{Object: mockConfigMap(util.VPNServerExposedPortKey, "127.0.0.4:80")}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update old object is not configmap",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: &unstructured.Unstructured{},
					ObjectNew: mockConfigMap(util.ProxyServerExposedPortKey, "127.0.0.3:90"),
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 0 when Update old object is not configmap",
			expectedLen: 0,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: mockConfigMap(util.ProxyServerExposedPortKey, "127.0.0.3:90"),
					ObjectNew: &unstructured.Unstructured{},
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update ConfigMap data is Proxy",
			expectedLen: 1,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: mockConfigMap(util.ProxyServerExposedPortKey, "127.0.0.3:80"),
					ObjectNew: mockConfigMap(util.ProxyServerExposedPortKey, "127.0.0.3:90"),
				}, queue)
			},
		},
		{
			name:        "should get work queue len is 1 when Update ConfigMap data is VPN",
			expectedLen: 1,
			eventHandler: func() {
				h.Update(context.Background(), event.UpdateEvent{
					ObjectOld: mockConfigMap(util.VPNServerExposedPortKey, "127.0.0.4:80"),
					ObjectNew: mockConfigMap(util.VPNServerExposedPortKey, "127.0.0.4:90"),
				}, queue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.eventHandler()
			assert.Equal(t, tt.expectedLen, queue.Len(), "Unexpected queue length")
			clearQueue(queue)
		})
	}
}

func mockObjs() []runtime.Object {
	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node1Name,
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
			},
		},
	}
	gateways := &ravenv1beta1.GatewayList{
		Items: []ravenv1beta1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: MockGateway,
				},
				Spec: ravenv1beta1.GatewaySpec{
					ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
				},
			},
		},
	}

	return []runtime.Object{nodeList, gateways, configmaps}
}

func clearQueue(queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	for queue.Len() > 0 {
		item, _ := queue.Get()
		queue.Done(item)
	}
}

func mockGateway(name string, exposeType string) *ravenv1beta1.Gateway {
	return &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: ravenv1beta1.GatewaySpec{
			ExposeType: exposeType,
		},
	}
}

func mockConfigMap(key, value string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "default",
		},
		Data: map[string]string{
			key: value,
		},
	}
}
