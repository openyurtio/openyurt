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

package gateway

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	"github.com/openyurtio/openyurt/pkg/controller/raven/utils"
)

type EnqueueGatewayForNode struct{}

// Create implements EventHandler
func (e *EnqueueGatewayForNode) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1.Node"))
		return
	}
	klog.V(5).Infof(Format("will enqueue gateway as node(%s) has been created",
		node.GetName()))
	if gwName, exist := node.Labels[raven.LabelCurrentGateway]; exist {
		utils.AddGatewayToWorkQueue(gwName, q)
		return
	}
	klog.V(4).Infof(Format("node(%s) does not belong to any gateway", node.GetName()))
}

// Update implements EventHandler
func (e *EnqueueGatewayForNode) Update(evt event.UpdateEvent,
	q workqueue.RateLimitingInterface) {
	newNode, ok := evt.ObjectNew.(*corev1.Node)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to v1.Node",
			evt.ObjectNew.GetName()))
		return
	}
	oldNode, ok := evt.ObjectOld.(*corev1.Node)
	if !ok {
		klog.Errorf(Format("fail to assert runtime Object(%s) to v1.Node",
			evt.ObjectOld.GetName()))
		return
	}
	klog.V(5).Infof(Format("Will enqueue gateway as node(%s) has been updated",
		newNode.GetName()))

	oldGwName := oldNode.Labels[raven.LabelCurrentGateway]
	newGwName := newNode.Labels[raven.LabelCurrentGateway]

	// check if NodeReady condition changed
	statusChanged := func(oldObj, newObj *corev1.Node) bool {
		return isNodeReady(*oldObj) != isNodeReady(*newObj)
	}

	if oldGwName != newGwName || statusChanged(oldNode, newNode) {
		utils.AddGatewayToWorkQueue(oldGwName, q)
		utils.AddGatewayToWorkQueue(newGwName, q)
	}
}

// Delete implements EventHandler
func (e *EnqueueGatewayForNode) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("Fail to assert runtime Object to v1.Node"))
		return
	}

	gwName, exist := node.Labels[raven.LabelCurrentGateway]
	if !exist {
		klog.V(5).Infof(Format("Node(%s) doesn't belong to any gateway", node.GetName()))
		return
	}
	// enqueue the gateway that the node belongs to
	klog.V(5).Infof(Format("Will enqueue pool(%s) as node(%s) has been deleted",
		gwName, node.GetName()))
	utils.AddGatewayToWorkQueue(gwName, q)
}

// Generic implements EventHandler
func (e *EnqueueGatewayForNode) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}
