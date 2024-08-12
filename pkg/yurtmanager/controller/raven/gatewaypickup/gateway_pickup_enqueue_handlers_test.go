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
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	MockGatewayUpdate = "gw-mock-update"
)

func mockNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-enqueue-gateway-for-node",
			Labels: map[string]string{
				raven.LabelCurrentGateway: MockGateway,
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
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string]string{
			util.RavenEnableProxy:  "true",
			util.RavenEnableTunnel: "true",
		},
	}
}

func TestEnqueueGatewayForNode(t *testing.T) {
	h := &EnqueueGatewayForNode{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	node := mockNode()
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}
	h.Create(context.Background(), event.CreateEvent{Object: node}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create node, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	time := metav1.Now()
	deletedNode := node.DeepCopy()
	deletedNode.DeletionTimestamp = &time
	h.Delete(context.Background(), event.DeleteEvent{Object: deletedNode}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to delete node, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newNodeLabel := node.DeepCopy()
	newNodeLabel.ObjectMeta.Labels[raven.LabelCurrentGateway] = MockGatewayUpdate
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: node, ObjectNew: newNodeLabel}, queue)
	if !assert.Equal(t, 2, queue.Len()) {
		t.Errorf("failed to update node gateway label, expected %d, but get %d", 2, queue.Len())
	}
	clearQueue(queue)

	newNodeStatus := node.DeepCopy()
	newNodeStatus.Status.Conditions[0].Status = corev1.ConditionFalse
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: node, ObjectNew: newNodeStatus}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update node status, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}

func TestEnqueueGatewayForRavenConfig(t *testing.T) {
	// h := &EnqueueGatewayForRavenConfig{}
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
					Name: MockGateway,
				},
			},
		},
	}

	objs := []runtime.Object{nodeList, gateways, configmaps}

	scheme := runtime.NewScheme()
	ravenv1beta1.AddToScheme(scheme)
	clientgoscheme.AddToScheme(scheme)

	h := EnqueueGatewayForRavenConfig{
		client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
	}

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

	time := metav1.Now()
	deletedConfigMap := configMap.DeepCopy()
	deletedConfigMap.DeletionTimestamp = &time
	h.Delete(context.Background(), event.DeleteEvent{Object: deletedConfigMap}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to delete config map, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newConfigMapProxy := configMap.DeepCopy()
	newConfigMapProxy.Data[util.RavenEnableProxy] = "false"
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMap, ObjectNew: newConfigMapProxy}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map proxy enable, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newConfigMapTunnel := configMap.DeepCopy()
	newConfigMapTunnel.Data[util.RavenEnableTunnel] = "false"
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: configMap, ObjectNew: newConfigMapTunnel}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update config map tunnel enable, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}
