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

package endpoints

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/adapter"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/util"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

type EnqueueEndpointsForService struct {
	endpointsAdapter adapter.Adapter
}

// Create implements EventHandler
func (e *EnqueueEndpointsForService) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
}

// Update implements EventHandler
func (e *EnqueueEndpointsForService) Update(evt event.UpdateEvent,
	q workqueue.RateLimitingInterface) {
	oldSvc, ok := evt.ObjectOld.(*corev1.Service)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to v1.Service",
			evt.ObjectOld.GetName()))
		return
	}
	newSvc, ok := evt.ObjectNew.(*corev1.Service)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to v1.Service",
			evt.ObjectNew.GetName()))
		return
	}
	if util.ServiceTopologyTypeChanged(oldSvc, newSvc) {
		e.enqueueEndpointsForSvc(newSvc, q)
	}
}

// Delete implements EventHandler
func (e *EnqueueEndpointsForService) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
}

// Generic implements EventHandler
func (e *EnqueueEndpointsForService) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}

func (e *EnqueueEndpointsForService) enqueueEndpointsForSvc(newSvc *corev1.Service, q workqueue.RateLimitingInterface) {
	keys := e.endpointsAdapter.GetEnqueueKeysBySvc(newSvc)
	klog.Infof(Format("the topology configuration of svc %s/%s is changed, enqueue endpoints: %v", newSvc.Namespace, newSvc.Name, keys))
	for _, key := range keys {
		ns, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			klog.Errorf("failed to split key %s, %v", key, err)
			continue
		}
		q.AddRateLimited(reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
		})
	}
}

type EnqueueEndpointsForNodePool struct {
	endpointsAdapter adapter.Adapter
	client           client.Client
}

// Create implements EventHandler
func (e *EnqueueEndpointsForNodePool) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
}

// Update implements EventHandler
func (e *EnqueueEndpointsForNodePool) Update(evt event.UpdateEvent,
	q workqueue.RateLimitingInterface) {
	oldNp, ok := evt.ObjectOld.(*nodepoolv1alpha1.NodePool)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to nodepoolv1alpha1.NodePool",
			evt.ObjectOld.GetName()))
		return
	}
	newNp, ok := evt.ObjectNew.(*nodepoolv1alpha1.NodePool)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to nodepoolv1alpha1.NodePool",
			evt.ObjectNew.GetName()))
		return
	}
	newNpNodes := sets.NewString(newNp.Status.Nodes...)
	oldNpNodes := sets.NewString(oldNp.Status.Nodes...)
	if newNpNodes.Equal(oldNpNodes) {
		return
	}
	klog.Infof(Format("the nodes record of nodepool %s is changed from %v to %v.", newNp.Name, oldNp.Status.Nodes, newNp.Status.Nodes))
	allNpNodes := newNpNodes.Union(oldNpNodes)
	svcTopologyTypes := util.GetSvcTopologyTypes(e.client)
	e.enqueueEndpointsForNodePool(svcTopologyTypes, allNpNodes, newNp, q)
}

// Delete implements EventHandler
func (e *EnqueueEndpointsForNodePool) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
	nodePool, ok := evt.Object.(*nodepoolv1alpha1.NodePool)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to nodepoolv1alpha1.NodePool",
			evt.Object.GetName()))
		return
	}
	klog.Infof(Format("nodepool %s is deleted", nodePool.Name))
	allNpNodes := sets.NewString(nodePool.Status.Nodes...)
	svcTopologyTypes := util.GetSvcTopologyTypes(e.client)
	e.enqueueEndpointsForNodePool(svcTopologyTypes, allNpNodes, nodePool, q)
}

// Generic implements EventHandler
func (e *EnqueueEndpointsForNodePool) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}

func (e *EnqueueEndpointsForNodePool) enqueueEndpointsForNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String, np *nodepoolv1alpha1.NodePool, q workqueue.RateLimitingInterface) {
	keys := e.endpointsAdapter.GetEnqueueKeysByNodePool(svcTopologyTypes, allNpNodes)
	klog.Infof(Format("according to the change of the nodepool %s, enqueue endpoints: %v", np.Name, keys))
	for _, key := range keys {
		ns, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			klog.Errorf("failed to split key %s, %v", key, err)
			continue
		}
		q.AddRateLimited(reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
		})
	}
}
