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

package gatewaypublicservice

import (
	"context"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueRequestForGatewayEvent struct{}

func (h *EnqueueRequestForGatewayEvent) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	gw, ok := e.Object.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway,", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if gw.Spec.ExposeType != ravenv1beta1.ExposeTypeLoadBalancer {
		return
	}
	klog.V(4).Info(Format("enqueue gateway %s as create event", gw.GetName()))
	util.AddGatewayToWorkQueue(gw.GetName(), q)
}

func (h *EnqueueRequestForGatewayEvent) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newGw, ok := e.ObjectNew.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway,", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()))
		return
	}
	oldGw, ok := e.ObjectOld.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway,", e.ObjectOld.GetNamespace(), e.ObjectOld.GetName()))
		return
	}
	if needUpdate(newGw, oldGw) {
		klog.V(4).Info(Format("enqueue gateway %s as update event", newGw.GetName()))
		util.AddGatewayToWorkQueue(newGw.GetName(), q)
	}
}

func (h *EnqueueRequestForGatewayEvent) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	gw, ok := e.Object.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway,", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if gw.Spec.ExposeType != ravenv1beta1.ExposeTypeLoadBalancer {
		return
	}
	klog.V(4).Info(Format("enqueue gateway %s as delete event", gw.GetName()))
	util.AddGatewayToWorkQueue(gw.GetName(), q)
}

func (h *EnqueueRequestForGatewayEvent) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

func needUpdate(newObj, oldObj *ravenv1beta1.Gateway) bool {
	if newObj.Spec.ExposeType == ravenv1beta1.ExposeTypeLoadBalancer || oldObj.Spec.ExposeType == ravenv1beta1.ExposeTypeLoadBalancer {
		if newObj.Spec.ExposeType != oldObj.Spec.ExposeType {
			return true
		}
		if util.HashObject(newObj.Status.ActiveEndpoints) != util.HashObject(oldObj.Status.ActiveEndpoints) {
			return true
		}
	}
	return false
}

type EnqueueRequestForConfigEvent struct {
	client client.Client
}

func (h *EnqueueRequestForConfigEvent) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	cm, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap,", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if cm.Data == nil {
		return
	}
	if _, _, err := net.SplitHostPort(cm.Data[util.ProxyServerExposedPortKey]); err == nil {
		addExposedGateway(h.client, q)
		return
	}
	if _, _, err := net.SplitHostPort(cm.Data[util.VPNServerExposedPortKey]); err == nil {
		addExposedGateway(h.client, q)
		return
	}
}

func (h *EnqueueRequestForConfigEvent) Update(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newCm, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap,", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()))
		return
	}
	oldCm, ok := e.ObjectOld.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap,", e.ObjectOld.GetNamespace(), e.ObjectOld.GetName()))
		return
	}
	_, newProxyPort, newErr := net.SplitHostPort(newCm.Data[util.ProxyServerExposedPortKey])
	_, oldProxyPort, oldErr := net.SplitHostPort(oldCm.Data[util.ProxyServerExposedPortKey])
	if newErr == nil && oldErr == nil && newProxyPort != oldProxyPort {
		addExposedGateway(h.client, q)
		return
	}
	_, newTunnelPort, newErr := net.SplitHostPort(newCm.Data[util.VPNServerExposedPortKey])
	_, oldTunnelPort, oldErr := net.SplitHostPort(oldCm.Data[util.VPNServerExposedPortKey])
	if newErr == nil && oldErr == nil && newTunnelPort != oldTunnelPort {
		addExposedGateway(h.client, q)
		return
	}
}

func (h *EnqueueRequestForConfigEvent) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueRequestForConfigEvent) Generic(ctx context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

func addExposedGateway(client client.Client, q workqueue.RateLimitingInterface) {
	var gwList ravenv1beta1.GatewayList
	err := client.List(context.TODO(), &gwList)
	if err != nil {
		return
	}
	for _, gw := range gwList.Items {
		if gw.Spec.ExposeType == ravenv1beta1.ExposeTypeLoadBalancer {
			klog.V(4).Info(Format("enqueue gateway %s", gw.GetName()))
			util.AddGatewayToWorkQueue(gw.GetName(), q)
		}
	}
}
