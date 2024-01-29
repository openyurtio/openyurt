package loadbalancer

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetLoadBalancerClass(svc *v1.Service) string {
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer && svc.Spec.LoadBalancerClass != nil {
		return *svc.Spec.LoadBalancerClass
	}
	return UnknownClass
}

func Key(obj client.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

func Type(obj client.Object) string {
	return obj.GetObjectKind().GroupVersionKind().Kind
}

func needDeleteLoadBalancer(svc *v1.Service) bool {
	return svc.DeletionTimestamp != nil || svc.Spec.Type != v1.ServiceTypeLoadBalancer
}
