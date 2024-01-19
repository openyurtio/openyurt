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

package v1beta1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

const YurtAppSetKind = "YurtAppSet"

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	set, ok := obj.(*v1beta1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", obj))
	}

	template := set.Spec.Workload.WorkloadTemplate
	if template.DeploymentTemplate == nil && template.StatefulSetTemplate == nil {
		return apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), set.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), template, "no workload template is configured")})
	} else if template.DeploymentTemplate != nil && template.StatefulSetTemplate != nil {
		return apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), set.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), template, "only one workload template should be configured")})
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newSet, ok := newObj.(*v1beta1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", newObj))
	}
	oldSet, ok := oldObj.(*v1beta1.YurtAppSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", oldObj))
	}

	newTemplate := newSet.Spec.Workload.WorkloadTemplate
	if newTemplate.DeploymentTemplate == nil && newTemplate.StatefulSetTemplate == nil {
		return apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "no workload template is configured")})
	} else if newTemplate.DeploymentTemplate != nil && newTemplate.StatefulSetTemplate != nil {
		return apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "only one workload template should be configured")})
	}

	oldTemplate := oldSet.Spec.Workload.WorkloadTemplate
	if (oldTemplate.DeploymentTemplate == nil && newTemplate.DeploymentTemplate != nil) ||
		(oldTemplate.StatefulSetTemplate == nil && newTemplate.StatefulSetTemplate != nil) {
		return apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "the kind of workload template should not be changed")})
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}
