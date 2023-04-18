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

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	appset, ok := obj.(*v1alpha1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", obj))
	}

	if allErrs := validateYurtAppSet(webhook.Client, appset); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("YurtAppSet").GroupKind(), appset.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newAppSet, ok := newObj.(*v1alpha1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", newObj))
	}
	oldAppSet, ok := oldObj.(*v1alpha1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", oldObj))
	}

	validationErrorList := validateYurtAppSet(webhook.Client, newAppSet)
	updateErrorList := ValidateYurtAppSetUpdate(newAppSet, oldAppSet)

	if allErrs := append(validationErrorList, updateErrorList...); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("YurtAppSet").GroupKind(), newAppSet.Name, allErrs)
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}
