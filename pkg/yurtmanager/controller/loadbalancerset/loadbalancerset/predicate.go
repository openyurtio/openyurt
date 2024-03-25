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
	"reflect"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

func NewServicePredicated() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			svc, ok := createEvent.Object.(*v1.Service)
			if !ok {
				return false
			}

			return isLoadBalancerSetService(svc)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldSvc, ok := event.ObjectOld.(*v1.Service)
			if !ok {
				return false
			}

			newSvc, ok := event.ObjectNew.(*v1.Service)
			if !ok {
				return false
			}

			if !isLoadBalancerSetService(oldSvc) && !isLoadBalancerSetService(newSvc) {
				return false
			}

			return isServiceChange(oldSvc, newSvc)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			svc, ok := deleteEvent.Object.(*v1.Service)
			if !ok {
				return false
			}

			return isLoadBalancerSetService(svc)
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			svc, ok := genericEvent.Object.(*v1.Service)
			if !ok {
				return false
			}

			return isLoadBalancerSetService(svc)
		},
	}
}

func isLoadBalancerSetService(svc *v1.Service) bool {
	if svc.Spec.Type != v1.ServiceTypeLoadBalancer {
		return false
	}

	if svc.Spec.LoadBalancerClass == nil {
		return false
	}

	if svc.Annotations == nil {
		return false
	}

	if len(svc.Annotations[network.AnnotationNodePoolSelector]) == 0 {
		return false
	}

	return true
}

func isServiceChange(oldSvc, newSvc *v1.Service) bool {
	if isDeleteTimeChange(oldSvc, newSvc) {
		return true
	}

	if isNodePoolSelectorChange(oldSvc.Annotations, newSvc.Annotations) {
		return true
	}

	if isServiceTypeChange(oldSvc, newSvc) {
		return true
	}

	if isFinalizersChange(oldSvc, newSvc) {
		return true
	}

	if isAggregateAnnotationsChange(oldSvc, newSvc) {
		return true
	}

	return false
}

func isDeleteTimeChange(oldSvc, newSvc *v1.Service) bool {
	return oldSvc.DeletionTimestamp != newSvc.DeletionTimestamp
}

func isNodePoolSelectorChange(oldAnnotations, newAnnotations map[string]string) bool {
	return !annotationValueIsEqual(oldAnnotations, newAnnotations, network.AnnotationNodePoolSelector)
}

func isServiceTypeChange(oldSvc, newSvc *v1.Service) bool {
	return oldSvc.Spec.Type != newSvc.Spec.Type
}

func isFinalizersChange(oldSvc, newSvc *v1.Service) bool {
	return !reflect.DeepEqual(oldSvc.Finalizers, newSvc.Finalizers)
}

func isAggregateAnnotationsChange(oldSvc, newSvc *v1.Service) bool {
	oldAggregateAnnotations := filterIgnoredKeys(oldSvc.Annotations)
	newAggregateAnnotations := filterIgnoredKeys(newSvc.Annotations)

	return !reflect.DeepEqual(oldAggregateAnnotations, newAggregateAnnotations)
}

func NewPoolServicePredicated() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			ps, ok := createEvent.Object.(*v1alpha1.PoolService)
			if !ok {
				return false
			}
			return predicatedPoolService(ps)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldPs, ok := updateEvent.ObjectOld.(*v1alpha1.PoolService)
			if !ok {
				return false
			}

			newPS, ok := updateEvent.ObjectNew.(*v1alpha1.PoolService)
			if !ok {
				return false
			}
			return predicatedPoolService(oldPs) ||
				predicatedPoolService(newPS)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			ps, ok := deleteEvent.Object.(*v1alpha1.PoolService)
			if !ok {
				return false
			}
			return predicatedPoolService(ps)
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			ps, ok := genericEvent.Object.(*v1alpha1.PoolService)
			if !ok {
				return false
			}
			return predicatedPoolService(ps)
		},
	}
}

func predicatedPoolService(ps *v1alpha1.PoolService) bool {
	if ps.Labels == nil {
		return false
	}

	if !hasServiceName(ps) {
		return false
	}

	return managedByController(ps)
}

func hasServiceName(ps *v1alpha1.PoolService) bool {
	if _, ok := ps.Labels[network.LabelServiceName]; !ok {
		return false
	}
	return true
}

func managedByController(ps *v1alpha1.PoolService) bool {
	return ps.Labels[labelManageBy] == names.LoadBalancerSetController
}

func NewNodePoolPredicated() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldNp, ok := updateEvent.ObjectOld.(*v1beta1.NodePool)
			if !ok {
				return false
			}

			newNp, ok := updateEvent.ObjectNew.(*v1beta1.NodePool)
			if !ok {
				return false
			}

			return isNodePoolChange(oldNp, newNp)
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			np, ok := createEvent.Object.(*v1beta1.NodePool)
			if !ok {
				return false
			}
			return nodePoolHasLabels(np)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			np, ok := deleteEvent.Object.(*v1beta1.NodePool)
			if !ok {
				return false
			}
			return nodePoolHasLabels(np)
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			np, ok := genericEvent.Object.(*v1beta1.NodePool)
			if !ok {
				return false
			}
			return nodePoolHasLabels(np)
		},
	}
}

func isNodePoolChange(oldNp, newNp *v1beta1.NodePool) bool {
	if !reflect.DeepEqual(oldNp.Labels, newNp.Labels) {
		return true
	}
	return false
}

func nodePoolHasLabels(np *v1beta1.NodePool) bool {
	return len(np.Labels) != 0
}
