/*
Copyright 2023 The OpenYurt Authors.

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
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

type EnqueueNodePoolForNode struct {
	EnableSyncNodePoolConfigurations bool
	Recorder                         record.EventRecorder
}

// Create implements EventHandler
func (e *EnqueueNodePoolForNode) Create(ctx context.Context, evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
		return
	}
	klog.V(5).Info(Format("will enqueue nodepool as node(%s) has been created",
		node.GetName()))
	if np := node.Labels[projectinfo.GetNodePoolLabel()]; len(np) != 0 {
		addNodePoolToWorkQueue(np, q)
		return
	}
	klog.V(4).Info(Format("node(%s) does not belong to any nodepool", node.GetName()))
}

// Update implements EventHandler
func (e *EnqueueNodePoolForNode) Update(ctx context.Context, evt event.UpdateEvent,
	q workqueue.RateLimitingInterface) {
	newNode, ok := evt.ObjectNew.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object(%s) to v1.Node",
			evt.ObjectNew.GetName()))
		return
	}
	oldNode, ok := evt.ObjectOld.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object(%s) to v1.Node",
			evt.ObjectOld.GetName()))
		return
	}

	newNp := newNode.Labels[projectinfo.GetNodePoolLabel()]
	oldNp := oldNode.Labels[projectinfo.GetNodePoolLabel()]

	// check the NodePoolLabel of node
	if len(oldNp) == 0 && len(newNp) == 0 {
		return
	} else if len(oldNp) == 0 {
		// add node to the new Pool
		klog.V(4).Info(Format("node(%s) is added into pool(%s)", newNode.Name, newNp))
		addNodePoolToWorkQueue(newNp, q)
		return
	} else if oldNp != newNp {
		klog.Warningf("It is not allowed to change the NodePoolLabel of node, but pool of node(%s) is changed from %s to %s", newNode.Name, oldNp, newNp)
		// emit a warning event
		e.Recorder.Event(newNode.DeepCopy(), corev1.EventTypeWarning, apps.NodePoolChangedEvent,
			fmt.Sprintf("It is not allowed to change the NodePoolLabel of node, but nodepool of node(%s) is changed from %s to %s", newNode.Name, oldNp, newNp))
		return
	}

	// check node ready status
	if isNodeReady(*newNode) != isNodeReady(*oldNode) {
		klog.V(4).Info(Format("Node ready status has been changed,"+
			" will enqueue pool(%s) for node(%s)", newNp, newNode.GetName()))
		addNodePoolToWorkQueue(newNp, q)
		return
	}

	// check node's labels, annotations or taints are updated or not
	if e.EnableSyncNodePoolConfigurations {
		if !reflect.DeepEqual(newNode.Labels, oldNode.Labels) ||
			!reflect.DeepEqual(newNode.Annotations, oldNode.Annotations) ||
			!reflect.DeepEqual(newNode.Spec.Taints, oldNode.Spec.Taints) {
			// TODO only consider the pool related attributes
			klog.V(5).Info(Format("NodePool related attributes has been changed,will enqueue pool(%s) for node(%s)", newNp, newNode.Name))
			addNodePoolToWorkQueue(newNp, q)
		}
	}
}

// Delete implements EventHandler
func (e *EnqueueNodePoolForNode) Delete(ctx context.Context, evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
		return
	}

	np := node.Labels[projectinfo.GetNodePoolLabel()]
	if len(np) == 0 {
		klog.V(4).Info(Format("A orphan node(%s) is removed", node.Name))
		return
	}
	// enqueue the nodepool that the node belongs to
	klog.V(4).Info(Format("Will enqueue pool(%s) as node(%s) has been deleted",
		np, node.GetName()))
	addNodePoolToWorkQueue(np, q)
}

// Generic implements EventHandler
func (e *EnqueueNodePoolForNode) Generic(ctx context.Context, evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}

// addNodePoolToWorkQueue adds the nodepool the reconciler's workqueue
func addNodePoolToWorkQueue(npName string,
	q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Name: npName},
	})
}
