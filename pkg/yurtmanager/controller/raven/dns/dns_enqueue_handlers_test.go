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

package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

func mockService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.WorkingNamespace,
			Name:      util.GatewayProxyInternalService,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: ProxyIP,
		},
	}
}

func mockNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: Node1Name,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: Node1Address,
				},
			},
		},
	}
}

func TestEnqueueRequestFoServiceEvent(t *testing.T) {
	h := &EnqueueRequestForServiceEvent{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	svc := mockService()
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}
	h.Create(event.CreateEvent{Object: svc}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update service, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	deletedSvc := svc.DeepCopy()
	time := metav1.Now()
	deletedSvc.DeletionTimestamp = &time
	h.Delete(event.DeleteEvent{Object: deletedSvc}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update service, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	newSvc := svc.DeepCopy()
	newSvc.Spec.ClusterIP = "0.0.0.0"
	h.Update(event.UpdateEvent{ObjectOld: svc, ObjectNew: newSvc}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to update service, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}

func TestEnqueueRequestForNodeEvent(t *testing.T) {
	h := &EnqueueRequestForNodeEvent{}
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	node := mockNode()
	clearQueue := func(queue workqueue.RateLimitingInterface) {
		for queue.Len() > 0 {
			item, _ := queue.Get()
			queue.Done(item)
		}
	}
	h.Create(event.CreateEvent{Object: node}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create node, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)

	time := metav1.Now()
	deletedNode := node.DeepCopy()
	deletedNode.DeletionTimestamp = &time
	h.Delete(event.DeleteEvent{Object: deletedNode}, queue)
	if !assert.Equal(t, 1, queue.Len()) {
		t.Errorf("failed to create node, expected %d, but get %d", 1, queue.Len())
	}
	clearQueue(queue)
}
