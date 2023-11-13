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
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/adapter"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/util"
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
		klog.Errorf(Format("could not assert runtime Object(%s) to v1.Service",
			evt.ObjectOld.GetName()))
		return
	}
	newSvc, ok := evt.ObjectNew.(*corev1.Service)
	if !ok {
		klog.Errorf(Format("could not assert runtime Object(%s) to v1.Service",
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
			klog.Errorf("could not split key %s, %v", key, err)
			continue
		}
		q.AddRateLimited(reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
		})
	}
}
