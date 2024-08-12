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
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	ProxyPortKey    = "1.1.1.3:80"
	ProxyPortKeyNew = "1.1.1.3:90"
	VPNPortKey      = "1.1.1.4:80"
	VPNPortKeyNew   = "1.1.1.4:90"
)

func mockGatewayPublicIP() *ravenv1beta1.Gateway {
	return &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: MockGateway,
		},
		Spec: ravenv1beta1.GatewaySpec{
			ExposeType: ravenv1beta1.ExposeTypePublicIP,
		},
	}
}

func mockGatewayLB() *ravenv1beta1.Gateway {
	return &ravenv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: MockGateway,
		},
		Spec: ravenv1beta1.GatewaySpec{
			ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
		},
	}
}

func mockConfigMapProxy() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string]string{
			util.ProxyServerExposedPortKey: ProxyPortKey,
		},
	}
}

func mockConfigMapVPN() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string]string{
			util.VPNServerExposedPortKey: VPNPortKey,
		},
	}
}

func TestEnqueueRequestForGatewayEvent(t *testing.T) {
	h := &EnqueueRequestForGatewayEvent{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}

	gwPublicIP := mockGatewayPublicIP()
	h.Create(context.Background(), event.CreateEvent{Object: gwPublicIP}, queue)
	if !assert.Equal(t, 0, queue.Len()) {
		t.Errorf("failed to create gateway with Public IP, expected %d, but get %d", 0, queue.Len())
	}
	clearQueue(queue)

	gwLB := mockGatewayLB()
	h.Create(context.Background(), event.CreateEvent{Object: gwLB}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create gateway with LoadBalancer, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	time := metav1.Now()
	deletedGwPublicIP := gwPublicIP.DeepCopy()
	deletedGwPublicIP.DeletionTimestamp = &time
	h.Delete(context.Background(), event.DeleteEvent{Object: deletedGwPublicIP}, queue)
	if !assert.Equal(t, 0, queue.Len()) {
		t.Errorf("failed to delete gateway with Public IP, expected %d, but get %d", 0, queue.Len())
	}
	clearQueue(queue)

	deletedGwLB := gwLB.DeepCopy()
	deletedGwLB.DeletionTimestamp = &time
	h.Delete(context.Background(), event.DeleteEvent{Object: deletedGwLB}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to delete gateway with LoadBalancer, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	h.Update(context.Background(), event.UpdateEvent{ObjectOld: gwPublicIP, ObjectNew: gwLB}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update gateway label, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	gwPublicIPNew := gwPublicIP.DeepCopy()
	gwPublicIPNew.Status.ActiveEndpoints = []*ravenv1beta1.Endpoint{
		{
			NodeName: "node-1",
			Type:     ravenv1beta1.Tunnel,
		},
	}
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: gwPublicIP, ObjectNew: gwLB}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update gateway label, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}

func TestEnqueueRequestForConfigEvent(t *testing.T) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}

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
					Name:      "foo",
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

	objs := []runtime.Object{nodeList, gateways, configmaps}

	scheme := runtime.NewScheme()
	ravenv1beta1.AddToScheme(scheme)
	clientgoscheme.AddToScheme(scheme)

	h := EnqueueRequestForConfigEvent{
		client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
	}

	configMapProxy := mockConfigMapProxy()
	h.Create(context.Background(), event.CreateEvent{Object: configMapProxy}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create config map with Proxy, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	configMapVPN := mockConfigMapVPN()
	h.Create(context.Background(), event.CreateEvent{Object: configMapVPN}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create config map with VPN, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newConfigMapProxy := configMapProxy.DeepCopy()
	newConfigMapProxy.Data[util.ProxyServerExposedPortKey] = ProxyPortKeyNew
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMapProxy, ObjectNew: newConfigMapProxy}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map with Proxy, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newConfigMapVPN := configMapVPN.DeepCopy()
	newConfigMapVPN.Data[util.VPNServerExposedPortKey] = VPNPortKeyNew
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMapVPN, ObjectNew: newConfigMapVPN}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map with VPN, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}
