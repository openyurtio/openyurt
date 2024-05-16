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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/apps"
	v1 "k8s.io/kubernetes/pkg/apis/apps/v1"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/workloadmanager"
)

const YurtAppSetKind = "YurtAppSet"

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	set, ok := obj.(*v1beta1.YurtAppSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", obj))
	}

	template := set.Spec.Workload.WorkloadTemplate
	if template.DeploymentTemplate == nil && template.StatefulSetTemplate == nil {
		return nil, apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), set.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), template, "no workload template is configured")})
	} else if template.DeploymentTemplate != nil && template.StatefulSetTemplate != nil {
		return nil, apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), set.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), template, "only one workload template should be configured")})
	}

	if template.DeploymentTemplate != nil {
		if err := webhook.validateDeployment(set); err != nil {
			return nil, err
		}
	} else if template.StatefulSetTemplate != nil {
		if err := webhook.validateStatefulSet(set); err != nil {
			return nil, err
		}
	}

	klog.Infof("Validate YurtAppSet %s successfully ...", klog.KObj(set))
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newSet, ok := newObj.(*v1beta1.YurtAppSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", newObj))
	}
	oldSet, ok := oldObj.(*v1beta1.YurtAppSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppSet but got a %T", oldObj))
	}

	newTemplate := newSet.Spec.Workload.WorkloadTemplate
	if newTemplate.DeploymentTemplate == nil && newTemplate.StatefulSetTemplate == nil {
		return nil, apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "no workload template is configured")})
	} else if newTemplate.DeploymentTemplate != nil && newTemplate.StatefulSetTemplate != nil {
		return nil, apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "only one workload template should be configured")})
	}

	if newTemplate.DeploymentTemplate != nil {
		if err := webhook.validateDeployment(newSet); err != nil {
			return nil, err
		}
	} else if newTemplate.StatefulSetTemplate != nil {
		if err := webhook.validateStatefulSet(newSet); err != nil {
			return nil, err
		}
	}

	oldTemplate := oldSet.Spec.Workload.WorkloadTemplate
	if (oldTemplate.DeploymentTemplate == nil && newTemplate.DeploymentTemplate != nil) ||
		(oldTemplate.StatefulSetTemplate == nil && newTemplate.StatefulSetTemplate != nil) {
		return nil, apierrors.NewInvalid(v1beta1.GroupVersion.WithKind(YurtAppSetKind).GroupKind(), newSet.Name,
			field.ErrorList{field.Invalid(field.NewPath("spec").Child("workload").Child("WorkloadTemplate"), newTemplate, "the kind of workload template should not be changed")})
	}

	return nil, nil
}

// TODO: move functions under k8s.io/kubernetes to pkg/util/kubernetes
func (webhook *YurtAppSetHandler) validateDeployment(yas *v1beta1.YurtAppSet) error {
	if len(yas.Spec.Workload.WorkloadTweaks) == 0 {
		deploy := &appsv1.Deployment{}
		deploy.Spec = *yas.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopy()
		webhook.Scheme.Default(deploy)
		out := &apps.Deployment{}
		if err := v1.Convert_v1_Deployment_To_apps_Deployment(deploy, out, nil); err != nil {
			return err
		}
		allErrs := appsvalidation.ValidateDeploymentSpec(&out.Spec, nil, field.NewPath("spec"), validation.PodValidationOptions{})
		if len(allErrs) != 0 {
			return allErrs.ToAggregate()
		}
		return nil
	}
	// Checking tweaks one by one, because if we test them all together,
	// we might miss one invalid tweak. And that tweak could only apply to a specific workload.
	for _, yasTweak := range yas.Spec.Workload.WorkloadTweaks {
		deploy := &appsv1.Deployment{}
		deploy.Spec = *yas.Spec.Workload.WorkloadTemplate.DeploymentTemplate.Spec.DeepCopy()
		if err := workloadmanager.ApplyTweaksToDeployment(deploy, []*v1beta1.Tweaks{&yasTweak.Tweaks}); err != nil {
			return err
		}
		webhook.Scheme.Default(deploy)
		out := &apps.Deployment{}
		if err := v1.Convert_v1_Deployment_To_apps_Deployment(deploy, out, nil); err != nil {
			return err
		}
		allErrs := appsvalidation.ValidateDeploymentSpec(&out.Spec, nil, field.NewPath("spec"), validation.PodValidationOptions{})
		if len(allErrs) != 0 {
			return allErrs.ToAggregate()
		}
	}
	return nil
}

func (webhook *YurtAppSetHandler) validateStatefulSet(yas *v1beta1.YurtAppSet) error {
	if len(yas.Spec.Workload.WorkloadTweaks) == 0 {
		state := &appsv1.StatefulSet{}
		state.Spec = *yas.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.DeepCopy()
		webhook.Scheme.Default(state)
		out := &apps.StatefulSet{}
		if err := v1.Convert_v1_StatefulSet_To_apps_StatefulSet(state, out, nil); err != nil {
			return err
		}
		allErrs := appsvalidation.ValidateStatefulSetSpec(&out.Spec, field.NewPath("spec"), validation.PodValidationOptions{})
		if len(allErrs) != 0 {
			return allErrs.ToAggregate()
		}
		return nil
	}
	// Same as validateDeployment
	for _, yasTweak := range yas.Spec.Workload.WorkloadTweaks {
		state := &appsv1.StatefulSet{}
		state.Spec = *yas.Spec.Workload.WorkloadTemplate.StatefulSetTemplate.Spec.DeepCopy()
		if err := workloadmanager.ApplyTweaksToStatefulSet(state, []*v1beta1.Tweaks{&yasTweak.Tweaks}); err != nil {
			return err
		}
		webhook.Scheme.Default(state)
		out := &apps.StatefulSet{}
		if err := v1.Convert_v1_StatefulSet_To_apps_StatefulSet(state, out, nil); err != nil {
			return err
		}
		allErrs := appsvalidation.ValidateStatefulSetSpec(&out.Spec, field.NewPath("spec"), validation.PodValidationOptions{})
		if len(allErrs) != 0 {
			return allErrs.ToAggregate()
		}
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppSetHandler) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
