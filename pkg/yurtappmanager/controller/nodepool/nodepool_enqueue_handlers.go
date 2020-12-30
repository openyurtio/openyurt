/*
Copyright 2020 The OpenYurt Authors.

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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/event"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

type EnqueueNodePoolForNode struct{}

// Create implements EventHandler
func (e *EnqueueNodePoolForNode) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error("fail to assert runtime Object to v1.Node")
		return
	}
	klog.V(5).Infof("will enqueue nodepool as node(%s) has been created",
		node.GetName())
	if np, exist := node.Labels[appsv1alpha1.LabelDesiredNodePool]; exist {
		addNodePoolToWorkQueue(np, q)
		return
	}
	klog.V(4).Infof("node(%s) does not belong to any nodepool", node.GetName())
}

// Update implements EventHandler
func (e *EnqueueNodePoolForNode) Update(evt event.UpdateEvent,
	q workqueue.RateLimitingInterface) {
	newNode, ok := evt.ObjectNew.(*corev1.Node)
	if !ok {
		klog.Errorf("fail to assert runtime Object(%s) to v1.Node",
			evt.MetaNew.GetName())
		return
	}
	oldNode, ok := evt.ObjectOld.(*corev1.Node)
	if !ok {
		klog.Errorf("fail to assert runtime Object(%s) to v1.Node",
			evt.MetaOld.GetName())
		return
	}
	klog.V(5).Infof("will enqueue nodepool as node(%s) has been updated",
		newNode.GetName())
	newNp := newNode.Labels[appsv1alpha1.LabelDesiredNodePool]
	oldNp := oldNode.Labels[appsv1alpha1.LabelCurrentNodePool]

	if newNp != oldNp {
		if newNp == "" {
			// remove node from old pool
			klog.V(5).Infof("will enqueue old pool(%s) for node(%s)",
				oldNp, newNode.GetName())
			addNodePoolToWorkQueue(oldNp, q)
			return
		}

		if oldNp == "" {
			// add node to the new Pool
			klog.V(5).Infof("will enqueue new pool(%s) for node(%s)",
				newNp, newNode.GetName())
			addNodePoolToWorkQueue(newNp, q)
			return
		}
		klog.V(5).Infof("will enqueue both new pool(%s) and"+
			" old pool(%s) for node(%s)",
			newNp, oldNp, newNode.GetName())
		addNodePoolToWorkQueue(oldNp, q)
		addNodePoolToWorkQueue(newNp, q)
		return
	}

	if isNodeReady(*newNode) != isNodeReady(*oldNode) {
		// if the newNode and oldNode status are different
		klog.V(5).Infof("node phase has been changed,"+
			" will enqueue pool(%s) for node(%s)", newNp, newNode.GetName())
		addNodePoolToWorkQueue(newNp, q)
		return
	}

	if !reflect.DeepEqual(newNode.Labels, oldNode.Labels) ||
		!reflect.DeepEqual(newNode.Annotations, oldNode.Annotations) ||
		!reflect.DeepEqual(newNode.Spec.Taints, oldNode.Spec.Taints) {
		// if node's labels, annotations or taints are updated
		// TODO only consider the pool realted attributes
		klog.V(5).Infof("nodepool related attributes has been changed,"+
			" will enqueue pool(%s) for node(%s)",
			newNp, newNode.GetName())
		addNodePoolToWorkQueue(newNp, q)
	}

}

// Delete implements EventHandler
func (e *EnqueueNodePoolForNode) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error("fail to assert runtime Object to v1.Node")
		return
	}

	np := node.Labels[appsv1alpha1.LabelCurrentNodePool]
	if np == "" {
		klog.V(5).Infof("node(%s) doesn't belong to any pool", node.GetName())
		return
	}
	// enqueue the nodepool that the node belongs to
	klog.V(5).Infof("will enqueue pool(%s) as node(%s) has been deleted",
		np, node.GetName())
	addNodePoolToWorkQueue(np, q)
}

// Generic implements EventHandler
func (e *EnqueueNodePoolForNode) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
	return
}
