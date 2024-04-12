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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	poolServiceFinalizer = "poolservice.openyurt.io/resources"
)

func (r *ReconcileLoadBalancerSet) addFinalizer(svc *corev1.Service) error {
	if controllerutil.ContainsFinalizer(svc, poolServiceFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(svc, poolServiceFinalizer)
	return r.Update(context.Background(), svc)
}

func (r *ReconcileLoadBalancerSet) removeFinalizer(svc *corev1.Service) error {
	if !controllerutil.ContainsFinalizer(svc, poolServiceFinalizer) {
		return nil
	}

	controllerutil.RemoveFinalizer(svc, poolServiceFinalizer)
	return r.Update(context.Background(), svc)
}
