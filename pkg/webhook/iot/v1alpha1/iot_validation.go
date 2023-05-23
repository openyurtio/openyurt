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

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/apis/device/v1alpha1"
	util "github.com/openyurtio/openyurt/pkg/controller/iot/utils"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *IoTHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	np, ok := obj.(*v1alpha1.IoT)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a IoT but got a %T", obj))
	}

	//validate
	if allErrs := webhook.validate(ctx, np); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("EdgeX").GroupKind(), np.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *IoTHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newNp, ok := newObj.(*v1alpha1.IoT)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a IoT but got a %T", newObj))
	}
	oldNp, ok := oldObj.(*v1alpha1.IoT)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a IoT} but got a %T", oldObj))
	}

	// validate
	newErrorList := webhook.validate(ctx, newNp)
	oldErrorList := webhook.validate(ctx, oldNp)
	if allErrs := append(newErrorList, oldErrorList...); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("IoT").GroupKind(), newNp.Name, allErrs)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *IoTHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func (webhook *IoTHandler) validate(ctx context.Context, iot *v1alpha1.IoT) field.ErrorList {

	// verify that the poolname nodepool
	if nodePoolErrs := webhook.validateIoTWithNodePools(ctx, iot); nodePoolErrs != nil {
		return nodePoolErrs
	}
	return nil
}

func (webhook *IoTHandler) validateIoTWithNodePools(ctx context.Context, iot *v1alpha1.IoT) field.ErrorList {
	// verify that the poolname is a right nodepool name
	nodePools := &unitv1alpha1.NodePoolList{}
	if err := webhook.Client.List(ctx, nodePools); err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), iot.Spec.PoolName, "can not list nodepools, cause"+err.Error()),
		}
	}
	ok := false
	for _, nodePool := range nodePools.Items {
		if nodePool.ObjectMeta.Name == iot.Spec.PoolName {
			ok = true
			break
		}
	}
	if !ok {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), iot.Spec.PoolName, "can not find the nodePool"),
		}
	}
	// verify that no other edgex in the nodepool
	var iots v1alpha1.IoTList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: iot.Spec.PoolName}
	if err := webhook.Client.List(ctx, &iots, listOptions); err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), iot.Spec.PoolName, "can not list edgexes, cause"+err.Error()),
		}
	}
	for _, other := range iots.Items {
		if iot.Name != other.Name {
			return field.ErrorList{
				field.Invalid(field.NewPath("spec", "poolName"), iot.Spec.PoolName, "already used by other edgex instance,"),
			}
		}
	}

	return nil

}
