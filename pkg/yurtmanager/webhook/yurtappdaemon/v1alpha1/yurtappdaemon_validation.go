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

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const (
	YurtAppDaemonKind = "YurtAppDaemon"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppDaemonHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	daemon, ok := obj.(*v1alpha1.YurtAppDaemon)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", obj))
	}

	if allErrs := validateYurtAppDaemon(webhook.Client, daemon); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(YurtAppDaemonKind).GroupKind(), daemon.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppDaemonHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newDaemon, ok := newObj.(*v1alpha1.YurtAppDaemon)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", newObj))
	}
	oldDaemon, ok := oldObj.(*v1alpha1.YurtAppDaemon)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", oldObj))
	}

	validationErrorList := validateYurtAppDaemon(webhook.Client, newDaemon)
	updateErrorList := ValidateYurtAppDaemonUpdate(newDaemon, oldDaemon)
	if allErrs := append(validationErrorList, updateErrorList...); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(YurtAppDaemonKind).GroupKind(), newDaemon.Name, allErrs)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppDaemonHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

// validateYurtAppDaemon validates a YurtAppDaemon.
func validateYurtAppDaemon(c client.Client, yad *v1alpha1.YurtAppDaemon) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&yad.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, validateYurtAppDaemonSpec(c, &yad.Spec, field.NewPath("spec"))...)
	return allErrs
}

// validateYurtAppDaemonSpec tests if required fields in the YurtAppDaemon spec are set.
func validateYurtAppDaemonSpec(c client.Client, spec *v1alpha1.YurtAppDaemonSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), ""))
	} else {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for statefulset"))
		}
	}

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, ""))
	} else {
		allErrs = append(allErrs, validateWorkLoadTemplate(&(spec.WorkloadTemplate), selector, fldPath.Child("template"))...)
	}

	return allErrs
}

func validateWorkLoadTemplate(template *v1alpha1.WorkloadTemplate, selector labels.Selector, fldPath *field.Path) field.ErrorList {

	allErrs := field.ErrorList{}

	var templateCount int
	if template.StatefulSetTemplate != nil {
		templateCount++
	}
	if template.DeploymentTemplate != nil {
		templateCount++
	}

	if templateCount < 1 {
		allErrs = append(allErrs, field.Required(fldPath, "should provide one of (statefulSetTemplate/deploymentTemplate)"))
	} else if templateCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, template, "should provide only one of (statefulSetTemplate/deploymentTemplate)"))
	}

	if template.StatefulSetTemplate != nil {
		labels := labels.Set(template.StatefulSetTemplate.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("statefulSetTemplate", "metadata", "labels"), template.StatefulSetTemplate.Labels, "`selector` does not match template `labels`"))
		}
		allErrs = append(allErrs, validateStatefulSet(template.StatefulSetTemplate, fldPath.Child("statefulSetTemplate"))...)
		sstemplate := template.StatefulSetTemplate.Spec.Template
		coreTemplate, err := convertPodTemplateSpec(&sstemplate)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), sstemplate, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("statefulSetTemplate", "spec", "template"), apivalidation.PodValidationOptions{})...)
	}

	if template.DeploymentTemplate != nil {
		labels := labels.Set(template.DeploymentTemplate.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("deploymentTemplate", "metadata", "labels"),
				template.DeploymentTemplate.Labels, "`selector` does not match template `labels`"))
		}
		allErrs = append(allErrs, validateDeployment(template.DeploymentTemplate, fldPath.Child("deploymentTemplate"))...)
		template := template.DeploymentTemplate.Spec.Template
		coreTemplate, err := convertPodTemplateSpec(&template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, validatePodTemplateSpec(coreTemplate, selector, fldPath.Child("deploymentTemplate", "spec", "template"))...)
		allErrs = append(allErrs, apivalidation.ValidatePodTemplateSpec(coreTemplate,
			fldPath.Child("deploymentTemplate", "spec", "template"), apivalidation.PodValidationOptions{})...)
	}

	return allErrs
}

// ValidateYurtAppDaemonUpdate tests if required fields in the YurtAppDaemon are set.
func ValidateYurtAppDaemonUpdate(yad, oldYad *v1alpha1.YurtAppDaemon) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&yad.ObjectMeta, &oldYad.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, validateYurtAppDaemonSpecUpdate(&yad.Spec, &oldYad.Spec, field.NewPath("spec"))...)
	return allErrs
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}

func validateYurtAppDaemonSpecUpdate(spec, oldSpec *v1alpha1.YurtAppDaemonSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateWorkloadTemplateUpdate(&spec.WorkloadTemplate, &oldSpec.WorkloadTemplate, fldPath.Child("template"))...)
	return allErrs
}

func validateWorkloadTemplateUpdate(template, oldTemplate *v1alpha1.WorkloadTemplate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if template.StatefulSetTemplate != nil && oldTemplate.StatefulSetTemplate != nil {
		allErrs = append(allErrs, validateStatefulSetUpdate(template.StatefulSetTemplate, oldTemplate.StatefulSetTemplate,
			fldPath.Child("statefulSetTemplate"))...)
	}
	if template.DeploymentTemplate != nil && oldTemplate.DeploymentTemplate != nil {
		allErrs = append(allErrs, validateDeploymentUpdate(template.DeploymentTemplate, oldTemplate.DeploymentTemplate,
			fldPath.Child("deploymentTemplate"))...)
	}
	return allErrs
}

func validatePodTemplateSpec(template *core.PodTemplateSpec, selector labels.Selector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if template == nil {
		allErrs = append(allErrs, field.Required(fldPath, ""))
	} else {
		if !selector.Empty() {
			// Verify that the Deployment selector matches the labels in template.
			labels := labels.Set(template.Labels)
			if !selector.Matches(labels) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("metadata", "labels"), template.Labels, "`selector` does not match template `labels`"))
			}
		}
	}
	return allErrs
}

func validateStatefulSet(statefulSet *v1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	/*
		if statefulSet.Spec.Replicas != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *statefulSet.Spec.Replicas, "replicas in statefulSetTemplate will not be used"))
		}
	*/
	if statefulSet.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate != nil &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "updateStrategy", "rollingUpdate", "partition"), *statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition, "partition in statefulSetTemplate will not be used"))
	}

	return allErrs
}

func validateDeployment(deployment *v1alpha1.DeploymentTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	/*
		if deployment.Spec.Replicas != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *deployment.Spec.Replicas, "replicas in deploymentTemplate will not be used"))
		}
	*/
	return allErrs
}

func validateDeploymentUpdate(deployment, oldDeployment *v1alpha1.DeploymentTemplateSpec,
	fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	restoreReplicas := deployment.Spec.Replicas
	deployment.Spec.Replicas = oldDeployment.Spec.Replicas

	restoreTemplate := deployment.Spec.Template
	deployment.Spec.Template = oldDeployment.Spec.Template

	restoreStrategy := deployment.Spec.Strategy
	deployment.Spec.Strategy = oldDeployment.Spec.Strategy

	if !apiequality.Semantic.DeepEqual(deployment.Spec, oldDeployment.Spec) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("spec"),
			"updates to deployTemplate spec for fields other than 'template', 'strategy' and 'replicas' are forbidden"))
	}
	deployment.Spec.Replicas = restoreReplicas
	deployment.Spec.Template = restoreTemplate
	deployment.Spec.Strategy = restoreStrategy

	if deployment.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*deployment.Spec.Replicas),
			fldPath.Child("spec", "replicas"))...)
	}
	return allErrs

}

func validateStatefulSetUpdate(statefulSet, oldStatefulSet *v1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	restoreReplicas := statefulSet.Spec.Replicas
	statefulSet.Spec.Replicas = oldStatefulSet.Spec.Replicas

	restoreTemplate := statefulSet.Spec.Template
	statefulSet.Spec.Template = oldStatefulSet.Spec.Template

	restoreStrategy := statefulSet.Spec.UpdateStrategy
	statefulSet.Spec.UpdateStrategy = oldStatefulSet.Spec.UpdateStrategy

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("spec"), "updates to statefulsetTemplate spec for fields other than 'template', and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy

	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), fldPath.Child("spec", "replicas"))...)
	}
	return allErrs
}
