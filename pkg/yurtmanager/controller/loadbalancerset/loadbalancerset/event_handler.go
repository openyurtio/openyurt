/*
Copyright 2024 The OpenYurt Authors.

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

package loadbalancerset

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func NewPoolServiceEventHandler() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(event event.CreateEvent, limitingInterface workqueue.RateLimitingInterface) {
			handlePoolServiceNormal(event.Object, limitingInterface)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent, limitingInterface workqueue.RateLimitingInterface) {
			handlePoolServiceUpdate(updateEvent.ObjectOld, updateEvent.ObjectNew, limitingInterface)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
			handlePoolServiceNormal(deleteEvent.Object, limitingInterface)
		},
		GenericFunc: func(genericEvent event.GenericEvent, limitingInterface workqueue.RateLimitingInterface) {
			handlePoolServiceNormal(genericEvent.Object, limitingInterface)
		},
	}
}

func handlePoolServiceNormal(event client.Object, q workqueue.RateLimitingInterface) {
	ps := event.(*v1alpha1.PoolService)
	serviceName := getServiceNameFromPoolService(ps)
	enqueueService(ps.Namespace, serviceName, q)
}

func getServiceNameFromPoolService(poolService *v1alpha1.PoolService) string {
	if poolService.Labels == nil {
		return ""
	}

	return poolService.Labels[network.LabelServiceName]
}

func enqueueService(namespace, serviceName string, q workqueue.RateLimitingInterface) {
	if len(serviceName) == 0 || len(namespace) == 0 {
		return
	}

	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: namespace, Name: serviceName},
	})
}

func handlePoolServiceUpdate(oldObject, newObject client.Object, q workqueue.RateLimitingInterface) {
	oldPs := oldObject.(*v1alpha1.PoolService)
	newPs := newObject.(*v1alpha1.PoolService)

	oldServiceName := getServiceNameFromPoolService(oldPs)
	newServiceName := getServiceNameFromPoolService(newPs)

	if oldServiceName != newServiceName {
		klog.Warningf("service name of %s/%s is changed from %s to %s", oldPs.Namespace, oldPs.Name, oldServiceName, newServiceName)
		enqueueService(oldPs.Namespace, oldServiceName, q)
		enqueueService(newPs.Namespace, newServiceName, q)
		return
	}
	enqueueService(newPs.Namespace, newServiceName, q)

	return
}

func NewNodePoolEventHandler(c client.Client) handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(createEvent event.CreateEvent, limitingInterface workqueue.RateLimitingInterface) {
			allLoadBalancerSetServicesEnqueue(c, limitingInterface)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent, limitingInterface workqueue.RateLimitingInterface) {
			allLoadBalancerSetServicesEnqueue(c, limitingInterface)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
			nodePoolRelatedServiceEnqueue(c, deleteEvent.Object, limitingInterface)
		},
		GenericFunc: func(genericEvent event.GenericEvent, limitingInterface workqueue.RateLimitingInterface) {
			nodePoolRelatedServiceEnqueue(c, genericEvent.Object, limitingInterface)
		},
	}
}

func allLoadBalancerSetServicesEnqueue(c client.Client, q workqueue.RateLimitingInterface) {
	services := &v1.ServiceList{}
	err := c.List(context.Background(), services)
	if err != nil {
		return
	}

	for _, svc := range services.Items {
		if !isLoadBalancerSetService(&svc) {
			continue
		}
		enqueueService(svc.Namespace, svc.Name, q)
	}
}

func nodePoolRelatedServiceEnqueue(c client.Client, object client.Object, q workqueue.RateLimitingInterface) {
	np := object.(*v1beta1.NodePool)
	poolServiceList := &v1alpha1.PoolServiceList{}

	listSelector := client.MatchingLabels{
		network.LabelNodePoolName: np.Name,
		labelManageBy:             names.LoadBalancerSetController}

	if err := c.List(context.Background(), poolServiceList, listSelector); err != nil {
		return
	}

	for _, item := range poolServiceList.Items {
		handlePoolServiceNormal(&item, q)
	}
}
