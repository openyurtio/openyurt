package loadbalancer

import (
	"context"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/driver"
)

type enqueueRequestForEndpointSliceEvent struct{}

var _ handler.EventHandler = (*enqueueRequestForEndpointSliceEvent)(nil)

func NewEnqueueRequestForEndpointSliceEvent() *enqueueRequestForEndpointSliceEvent {
	return &enqueueRequestForEndpointSliceEvent{}
}

func (e *enqueueRequestForEndpointSliceEvent) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	enqueueServiceForEndpointSlice(evt.Object, q)

}

func (e *enqueueRequestForEndpointSliceEvent) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	enqueueServiceForEndpointSlice(evt.ObjectNew, q)
}

func (e *enqueueRequestForEndpointSliceEvent) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	enqueueServiceForEndpointSlice(evt.Object, q)
}

func (e *enqueueRequestForEndpointSliceEvent) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	enqueueServiceForEndpointSlice(evt.Object, q)
}

func enqueueServiceForEndpointSlice(obj client.Object, q workqueue.RateLimitingInterface) {
	es, ok := obj.(*discovery.EndpointSlice)
	if ok {
		if svcName, ok := es.Labels[discovery.LabelServiceName]; ok {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: es.Namespace, Name: svcName}})
		}
	}
}

type enqueueRequestForNode struct {
	client  client.Client
	drivers driver.Drivers
}

var _ handler.EventHandler = (*enqueueRequestForNode)(nil)

func NewEnqueueRequestForNodeEvent(client client.Client, drivers driver.Drivers) *enqueueRequestForNode {
	return &enqueueRequestForNode{client: client, drivers: drivers}
}

func (e *enqueueRequestForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

func (e *enqueueRequestForNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.ObjectNew), Type(evt.ObjectNew))
}

func (e *enqueueRequestForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

func (e *enqueueRequestForNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

type enqueueRequestForNodePool struct {
	client  client.Client
	drivers driver.Drivers
}

var _ handler.EventHandler = (*enqueueRequestForNodePool)(nil)

func NewEnqueueRequestForNodePoolEvent(client client.Client, drivers driver.Drivers) *enqueueRequestForNodePool {
	return &enqueueRequestForNodePool{client: client, drivers: drivers}
}

func (e *enqueueRequestForNodePool) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

func (e *enqueueRequestForNodePool) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.ObjectNew), Type(evt.ObjectNew))
}

func (e *enqueueRequestForNodePool) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

func (e *enqueueRequestForNodePool) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	enqueueLoadBalancerService(e.client, e.drivers, q, Key(evt.Object), Type(evt.Object))
}

func enqueueLoadBalancerService(client client.Client, drivers driver.Drivers, q workqueue.RateLimitingInterface, objectKey string, objectType string) {
	var svcList v1.ServiceList
	err := client.List(context.TODO(), &svcList)
	if err != nil {
		klog.Error(err, "fail to list services for object", "object type: ", objectKey, "object key: ", objectType)
		return
	}
	for _, svc := range svcList.Items {
		if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
			continue
		}
		if _, ok := drivers[GetLoadBalancerClass(&svc)]; !ok {
			continue
		}
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}})
	}
}
