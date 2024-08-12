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
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	MockGatewayUpdate    = "gw-mock-update"
	InsecurePortKey      = "1.1.1.1:80"
	InsecurePortKeyError = "1.1.1.1"
	InsecurePortKeyNew   = "1.1.1.1:90"
	SecurePortKey        = "1.1.1.2:80"
	SecurePortKeyNew     = "1.1.1.2:90"
)

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
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string]string{
			util.RavenEnableProxy:           "true",
			util.RavenEnableTunnel:          "true",
			util.ProxyServerInsecurePortKey: InsecurePortKey,
			util.ProxyServerSecurePortKey:   SecurePortKey,
		},
	}
}

func TestEnqueueRequestForGatewayEvent(t *testing.T) {
	h := &EnqueueRequestForGatewayEvent{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	gw := mockGateway()
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}
	h.Create(context.Background(), event.CreateEvent{Object: gw}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create gateway, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	time := metav1.Now()
	deletedGw := gw.DeepCopy()
	deletedGw.DeletionTimestamp = &time
	h.Delete(context.Background(), event.DeleteEvent{Object: deletedGw}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to delete gateway, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newGw := gw.DeepCopy()
	newGw.ObjectMeta.Name = MockGatewayUpdate
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: gw, ObjectNew: newGw}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update gateway label, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}

func TestEnqueueRequestForConfigEvent(t *testing.T) {
	h := &EnqueueRequestForConfigEvent{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	configMap := mockConfigMap()
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}
	h.Create(context.Background(), event.CreateEvent{Object: configMap}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create config map, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	configMap.Data[util.ProxyServerInsecurePortKey] = InsecurePortKeyError
	h.Create(context.Background(), event.CreateEvent{Object: configMap}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create config map, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
	configMap.Data[util.ProxyServerInsecurePortKey] = InsecurePortKey

	newConfigMap := configMap.DeepCopy()
	newConfigMap.Data[util.ProxyServerInsecurePortKey] = InsecurePortKeyNew
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMap, ObjectNew: newConfigMap}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newConfigMap = configMap.DeepCopy()
	newConfigMap.Data[util.ProxyServerSecurePortKey] = SecurePortKeyNew
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMap, ObjectNew: newConfigMap}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}
