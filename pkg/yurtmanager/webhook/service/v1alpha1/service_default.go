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

	viplb "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

// Default satisfies the defaulting webhook interface.
func (webhook *ServiceHandler) Default(ctx context.Context, obj runtime.Object) error {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Service but got a %T", obj))
	}

	if svc.Spec.LoadBalancerClass != nil {
		if *svc.Spec.LoadBalancerClass == viplb.VipLoadBalancerClass {
			if svc.Annotations == nil {
				svc.Annotations = make(map[string]string)
			}

			svc.Annotations[viplb.AnnotationServiceTopologyKey] = viplb.AnnotationServiceTopologyValueNodePool
		}

	}

	return nil
}
