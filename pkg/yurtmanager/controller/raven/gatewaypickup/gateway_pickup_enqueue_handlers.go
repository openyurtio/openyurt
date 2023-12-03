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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueGatewayForNode struct{}

// Create implements EventHandler
func (e *EnqueueGatewayForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
		return
	}
	klog.V(5).Infof(Format("will enqueue gateway as node(%s) has been created",
		node.GetName()))
	if gwName, exist := node.Labels[raven.LabelCurrentGateway]; exist {
		util.AddGatewayToWorkQueue(gwName, q)
		return
	}
	klog.V(4).Infof(Format("node(%s) does not belong to any gateway", node.GetName()))
}

// Update implements EventHandler
func (e *EnqueueGatewayForNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newNode, ok := evt.ObjectNew.(*corev1.Node)
	if !ok {
		klog.Errorf(Format("could not assert runtime Object(%s) to v1.Node",
			evt.ObjectNew.GetName()))
		return
	}
	oldNode, ok := evt.ObjectOld.(*corev1.Node)
	if !ok {
		klog.Errorf(Format("could not assert runtime Object(%s) to v1.Node",
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
		util.AddGatewayToWorkQueue(oldGwName, q)
		util.AddGatewayToWorkQueue(newGwName, q)
	}
}

// Delete implements EventHandler
func (e *EnqueueGatewayForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	node, ok := evt.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
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
	util.AddGatewayToWorkQueue(gwName, q)
}

// Generic implements EventHandler
func (e *EnqueueGatewayForNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

type EnqueueGatewayForRavenConfig struct {
	client client.Client
}

func (e *EnqueueGatewayForRavenConfig) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	_, ok := evt.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.ConfigMap"))
		return
	}
	klog.V(2).Infof(Format("Will config all gateway as raven-cfg has been created"))
	if err := e.enqueueGateways(q); err != nil {
		klog.Error(Format("could not config all gateway, error %s", err.Error()))
		return
	}
}

func (e *EnqueueGatewayForRavenConfig) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldCm, ok := evt.ObjectOld.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.ConfigMap"))
		return
	}

	newCm, ok := evt.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.ConfigMap"))
		return
	}

	if oldCm.Data[util.RavenEnableProxy] != newCm.Data[util.RavenEnableProxy] {
		klog.V(4).Infof(Format("Will config all gateway as raven-cfg has been updated"))
		if err := e.enqueueGateways(q); err != nil {
			klog.Error(Format("could not config all gateway, error %s", err.Error()))
			return
		}
	}

	if oldCm.Data[util.RavenEnableTunnel] != newCm.Data[util.RavenEnableTunnel] {
		klog.V(4).Infof(Format("Will config all gateway as raven-cfg has been updated"))
		if err := e.enqueueGateways(q); err != nil {
			klog.Error(Format("could not config all gateway, error %s", err.Error()))
			return
		}
	}
}

func (e *EnqueueGatewayForRavenConfig) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	_, ok := evt.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.ConfigMap"))
		return
	}
	klog.V(4).Infof(Format("Will config all gateway as raven-cfg has been deleted"))
	if err := e.enqueueGateways(q); err != nil {
		klog.Error(Format("could not config all gateway, error %s", err.Error()))
		return
	}
}

func (e *EnqueueGatewayForRavenConfig) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

func (e *EnqueueGatewayForRavenConfig) enqueueGateways(q workqueue.RateLimitingInterface) error {
	var gwList ravenv1beta1.GatewayList
	err := e.client.List(context.TODO(), &gwList)
	if err != nil {
		return err
	}
	for _, gw := range gwList.Items {
		util.AddGatewayToWorkQueue(gw.Name, q)
	}
	return nil
}
