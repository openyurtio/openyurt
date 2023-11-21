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

package gatewayinternalservice

import (
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueRequestForGatewayEvent struct{}

func (h *EnqueueRequestForGatewayEvent) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	gw, ok := e.Object.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if gw.Spec.ExposeType == "" {
		return
	}
	klog.V(4).Infof(Format("enqueue service %s/%s due to gateway %s create event", util.WorkingNamespace, util.GatewayProxyInternalService, gw.GetName()))
	util.AddGatewayProxyInternalService(q)
}

func (h *EnqueueRequestForGatewayEvent) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newGw, ok := e.ObjectNew.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()))
		return
	}
	oldGw, ok := e.ObjectOld.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway", e.ObjectOld.GetNamespace(), e.ObjectOld.GetName()))
		return
	}
	if oldGw.Spec.ExposeType == "" && newGw.Spec.ExposeType == "" {
		return
	}
	klog.V(4).Infof(Format("enqueue service %s/%s due to gateway %s update event", util.WorkingNamespace, util.GatewayProxyInternalService, newGw.GetName()))
	util.AddGatewayProxyInternalService(q)
}

func (h *EnqueueRequestForGatewayEvent) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	gw, ok := e.Object.(*ravenv1beta1.Gateway)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1beta1.Gateway", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if gw.Spec.ExposeType == "" {
		return
	}
	klog.V(4).Infof(Format("enqueue service %s/%s due to gateway %s delete event", util.WorkingNamespace, util.GatewayProxyInternalService, gw.GetName()))
	util.AddGatewayProxyInternalService(q)
}

func (h *EnqueueRequestForGatewayEvent) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

type EnqueueRequestForConfigEvent struct{}

func (h *EnqueueRequestForConfigEvent) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	cm, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap", e.Object.GetNamespace(), e.Object.GetName()))
		return
	}
	if cm.Data == nil {
		return
	}
	_, _, err := net.SplitHostPort(cm.Data[util.ProxyServerInsecurePortKey])
	if err == nil {
		klog.V(4).Infof(Format("enqueue service %s/%s due to config %s/%s create event",
			util.WorkingNamespace, util.GatewayProxyInternalService, util.WorkingNamespace, util.RavenAgentConfig))
		util.AddGatewayProxyInternalService(q)
		return
	}
	_, _, err = net.SplitHostPort(cm.Data[util.ProxyServerSecurePortKey])
	if err == nil {
		klog.V(4).Infof(Format("enqueue service %s/%s due to config %s/%s create event",
			util.WorkingNamespace, util.GatewayProxyInternalService, util.WorkingNamespace, util.RavenAgentConfig))
		util.AddGatewayProxyInternalService(q)
		return
	}
}

func (h *EnqueueRequestForConfigEvent) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newCm, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()))
		return
	}
	oldCm, ok := e.ObjectOld.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("could not assert runtime Object %s/%s to v1.Configmap", e.ObjectOld.GetNamespace(), e.ObjectOld.GetName()))
		return
	}
	_, newInsecurePort, newErr := net.SplitHostPort(newCm.Data[util.ProxyServerInsecurePortKey])
	_, oldInsecurePort, oldErr := net.SplitHostPort(oldCm.Data[util.ProxyServerInsecurePortKey])
	if newErr == nil && oldErr == nil {
		if newInsecurePort != oldInsecurePort {
			klog.V(4).Infof(Format("enqueue service %s/%s due to config %s/%s update event",
				util.WorkingNamespace, util.GatewayProxyInternalService, util.WorkingNamespace, util.RavenAgentConfig))
			util.AddGatewayProxyInternalService(q)
			return
		}
	}
	_, newSecurePort, newErr := net.SplitHostPort(newCm.Data[util.ProxyServerSecurePortKey])
	_, oldSecurePort, oldErr := net.SplitHostPort(oldCm.Data[util.ProxyServerSecurePortKey])
	if newErr == nil && oldErr == nil {
		if newSecurePort != oldSecurePort {
			klog.V(4).Infof(Format("enqueue service %s/%s due to config %s/%s update event",
				util.WorkingNamespace, util.GatewayProxyInternalService, util.WorkingNamespace, util.RavenAgentConfig))
			util.AddGatewayProxyInternalService(q)
			return
		}
	}
}

func (h *EnqueueRequestForConfigEvent) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueRequestForConfigEvent) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}
