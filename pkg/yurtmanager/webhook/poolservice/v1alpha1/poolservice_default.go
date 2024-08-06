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

package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	viplb "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

// Default satisfies the defaulting webhook interface.
func (webhook *PoolServiceHandler) Default(ctx context.Context, obj runtime.Object) error {
	ps, ok := obj.(*v1alpha1.PoolService)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a PoolService but got a %T", obj))
	}

	v1alpha1.SetDefaultsPoolService(ps)

	if ps.Spec.LoadBalancerClass != nil {
		if *ps.Spec.LoadBalancerClass == viplb.VipLoadBalancerClass {
			service := &corev1.Service{}
			svcName := ps.Labels[network.LabelServiceName]
			if err := webhook.Client.Get(ctx, types.NamespacedName{Name: svcName, Namespace: ps.Namespace}, service); err != nil {
				return apierrors.NewBadRequest(fmt.Sprintf("failed to get service %s corresponding to poolservice %s: %v", svcName, ps.Name, err))
			}

			if service.Annotations == nil {
				service.Annotations = make(map[string]string)
			}

			service.Annotations[viplb.AnnotationServiceTopologyKey] = viplb.AnnotationServiceTopologyValueNodePool

			if err := webhook.Client.Update(ctx, service); err != nil {
				return err
			}
		}

	}

	return nil
}
