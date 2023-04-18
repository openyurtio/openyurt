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

package endpointslice

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/adapter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	nodepoolv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

type EnqueueEndpointsliceForService struct {
	endpointsliceAdapter adapter.Adapter
}

// Create implements EventHandler
func (e *EnqueueEndpointsliceForService) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
}

// Update implements EventHandler
func (e *EnqueueEndpointsliceForService) Update(evt event.UpdateEvent,
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
	if isServiceTopologyTypeChanged(oldSvc, newSvc) {
		e.enqueueEndpointsliceForSvc(newSvc, q)
	}
}

// Delete implements EventHandler
func (e *EnqueueEndpointsliceForService) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
}

// Generic implements EventHandler
func (e *EnqueueEndpointsliceForService) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}

func (e *EnqueueEndpointsliceForService) enqueueEndpointsliceForSvc(newSvc *corev1.Service, q workqueue.RateLimitingInterface) {
	keys := e.endpointsliceAdapter.GetEnqueueKeysBySvc(newSvc)
	klog.Infof(Format("the topology configuration of svc %s/%s is changed, enqueue endpointslices: %v", newSvc.Namespace, newSvc.Name, keys))
	for _, key := range keys {
		q.AddRateLimited(key)
	}
}

func isServiceTopologyTypeChanged(oldSvc, newSvc *corev1.Service) bool {
	oldType := oldSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	newType := newSvc.Annotations[servicetopology.AnnotationServiceTopologyKey]
	if oldType == newType {
		return false
	}
	return true
}

type EnqueueEndpointsliceForNodePool struct {
	endpointsliceAdapter adapter.Adapter
	client               client.Client
}

// Create implements EventHandler
func (e *EnqueueEndpointsliceForNodePool) Create(evt event.CreateEvent,
	q workqueue.RateLimitingInterface) {
}

// Update implements EventHandler
func (e *EnqueueEndpointsliceForNodePool) Update(evt event.UpdateEvent,
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
	svcTopologyTypes := e.getSvcTopologyTypes()
	e.enqueueEndpointsliceForNodePool(svcTopologyTypes, allNpNodes, newNp, q)
}

// Delete implements EventHandler
func (e *EnqueueEndpointsliceForNodePool) Delete(evt event.DeleteEvent,
	q workqueue.RateLimitingInterface) {
	nodePool, ok := evt.Object.(*nodepoolv1alpha1.NodePool)
	if !ok {
		klog.Errorf(Format("Fail to assert runtime Object(%s) to nodepoolv1alpha1.NodePool",
			evt.Object.GetName()))
		return
	}
	klog.Infof(Format("nodepool %s is deleted", nodePool.Name))
	allNpNodes := sets.NewString(nodePool.Status.Nodes...)
	svcTopologyTypes := e.getSvcTopologyTypes()
	e.enqueueEndpointsliceForNodePool(svcTopologyTypes, allNpNodes, nodePool, q)
}

// Generic implements EventHandler
func (e *EnqueueEndpointsliceForNodePool) Generic(evt event.GenericEvent,
	q workqueue.RateLimitingInterface) {
}

func (e *EnqueueEndpointsliceForNodePool) enqueueEndpointsliceForNodePool(svcTopologyTypes map[string]string, allNpNodes sets.String, np *nodepoolv1alpha1.NodePool, q workqueue.RateLimitingInterface) {
	keys := e.endpointsliceAdapter.GetEnqueueKeysByNodePool(svcTopologyTypes, allNpNodes)
	klog.Infof(Format("according to the change of the nodepool %s, enqueue endpointslice: %v", np.Name, keys))
	for _, key := range keys {
		q.AddRateLimited(key)
	}
}

func (e *EnqueueEndpointsliceForNodePool) getSvcTopologyTypes() map[string]string {
	svcTopologyTypes := make(map[string]string)
	svcList := &corev1.ServiceList{}
	if err := e.client.List(context.TODO(), svcList, &client.ListOptions{LabelSelector: labels.Everything()}); err != nil {
		klog.V(4).Infof(Format("Error listing service sets: %v", err))
		return svcTopologyTypes
	}

	for _, svc := range svcList.Items {
		topologyType, ok := svc.Annotations[servicetopology.AnnotationServiceTopologyKey]
		if !ok {
			continue
		}

		key, err := cache.MetaNamespaceKeyFunc(svc)
		if err != nil {
			runtime.HandleError(err)
			continue
		}
		svcTopologyTypes[key] = topologyType
	}
	return svcTopologyTypes
}
