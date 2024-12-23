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
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueRequestForServiceEvent struct{}

func (h *EnqueueRequestForServiceEvent) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	svc, ok := e.Object.(*corev1.Service)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Service"))
		return
	}
	if svc.Spec.ClusterIP == "" {
		klog.Error(Format("could not get cluster IP %s/%s", svc.Namespace, svc.Name))
		return
	}

	klog.V(4).Info(Format("enqueue configmap %s/%s due to service create event", util.WorkingNamespace, util.RavenProxyNodesConfig))
	util.AddDNSConfigmapToWorkQueue(q)
}

func (h *EnqueueRequestForServiceEvent) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newSvc, ok := e.ObjectNew.(*corev1.Service)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Service"))
		return
	}
	oldSvc, ok := e.ObjectOld.(*corev1.Service)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Service"))
		return
	}
	if newSvc.Spec.ClusterIP != oldSvc.Spec.ClusterIP {
		klog.V(4).Info(Format("enqueue configmap %s/%s due to service update event", util.WorkingNamespace, util.RavenProxyNodesConfig))
		util.AddDNSConfigmapToWorkQueue(q)
	}
}

func (h *EnqueueRequestForServiceEvent) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	_, ok := e.Object.(*corev1.Service)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Service"))
		return
	}
	klog.V(4).Info(Format("enqueue configmap %s/%s due to service update event", util.WorkingNamespace, util.RavenProxyNodesConfig))
	util.AddDNSConfigmapToWorkQueue(q)
	return
}

func (h *EnqueueRequestForServiceEvent) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

type EnqueueRequestForNodeEvent struct{}

func (h *EnqueueRequestForNodeEvent) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	_, ok := e.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
		return
	}
	klog.V(4).Info(Format("enqueue configmap %s/%s due to node create event", util.WorkingNamespace, util.RavenProxyNodesConfig))
	util.AddDNSConfigmapToWorkQueue(q)
}

func (h *EnqueueRequestForNodeEvent) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueRequestForNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	_, ok := e.Object.(*corev1.Node)
	if !ok {
		klog.Error(Format("could not assert runtime Object to v1.Node"))
		return
	}
	klog.V(4).Info(Format("enqueue configmap %s/%s due to node delete event", util.WorkingNamespace, util.RavenProxyNodesConfig))
	util.AddDNSConfigmapToWorkQueue(q)
}

func (h *EnqueueRequestForNodeEvent) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {

}
