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

package viploadbalancer

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

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
	return matchLoadBalancerClass(ps)
}

func matchLoadBalancerClass(ps *v1alpha1.PoolService) bool {
	if ps.Spec.LoadBalancerClass == nil {
		return false
	}

	if *ps.Spec.LoadBalancerClass != VipLoadBalancerClass {
		return false
	}

	return true
}
