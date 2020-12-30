/*
Copyright 2020 The OpenYurt Authors.
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validating

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	unitv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

// ValidateUnitedDeploymentSpec tests if required fields in the UnitedDeployment spec are set.
func validateUnitedDeploymentSpec(c client.Client, spec *unitv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
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
		allErrs = append(allErrs, validatePoolTemplate(&(spec.WorkloadTemplate), spec, selector, fldPath.Child("template"))...)
	}

	poolNames := sets.String{}
	for i, pool := range spec.Topology.Pools {
		if len(pool.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("topology", "pools").Index(i).Child("name"), ""))
		}

		if poolNames.Has(pool.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "pools").Index(i).Child("name"), pool.Name,
				fmt.Sprintf("duplicated pool name %s", pool.Name)))
		}

		poolNames.Insert(pool.Name)
		if errs := apimachineryvalidation.NameIsDNSLabel(pool.Name, false); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "pools").Index(i).Child("name"), pool.Name,
				fmt.Sprintf("invalid pool name %s", strings.Join(errs, ", "))))
		}

		coreNodeSelectorTerm := &core.NodeSelectorTerm{}
		if err := corev1.Convert_v1_NodeSelectorTerm_To_core_NodeSelectorTerm(pool.NodeSelectorTerm.DeepCopy(), coreNodeSelectorTerm, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "pools").Index(i).Child("nodeSelectorTerm"),
				pool.NodeSelectorTerm, fmt.Sprintf("Convert_v1_NodeSelectorTerm_To_core_NodeSelectorTerm failed: %v", err)))
		} else {
			allErrs = append(allErrs, apivalidation.ValidateNodeSelectorTerm(*coreNodeSelectorTerm, fldPath.Child("topology", "pools").Index(i).Child("nodeSelectorTerm"))...)
		}

		if pool.Tolerations != nil {
			var coreTolerations []core.Toleration
			for i, toleration := range pool.Tolerations {
				coreToleration := &core.Toleration{}
				if err := corev1.Convert_v1_Toleration_To_core_Toleration(&toleration, coreToleration, nil); err != nil {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "pools").Index(i).Child("tolerations"), pool.Tolerations,
						fmt.Sprintf("Convert_v1_Toleration_To_core_Toleration failed: %v", err)))
				} else {
					coreTolerations = append(coreTolerations, *coreToleration)
				}
			}
			allErrs = append(allErrs, apivalidation.ValidateTolerations(coreTolerations, fldPath.Child("topology", "pools").Index(i).Child("tolerations"))...)
		}

	}

	return allErrs
}

// validateUnitedDeployment validates a UnitedDeployment.
func validateUnitedDeployment(c client.Client, unitedDeployment *unitv1alpha1.UnitedDeployment) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&unitedDeployment.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, validateUnitedDeploymentSpec(c, &unitedDeployment.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateUnitedDeploymentUpdate tests if required fields in the UnitedDeployment are set.
func ValidateUnitedDeploymentUpdate(unitedDeployment, oldUnitedDeployment *unitv1alpha1.UnitedDeployment) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&unitedDeployment.ObjectMeta, &oldUnitedDeployment.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, validateUnitedDeploymentSpecUpdate(&unitedDeployment.Spec, &oldUnitedDeployment.Spec, field.NewPath("spec"))...)
	return allErrs
}

func convertPodSpec(spec *v1.PodSpec) (*core.PodSpec, error) {
	coreSpec := &core.PodSpec{}
	if err := corev1.Convert_v1_PodSpec_To_core_PodSpec(spec.DeepCopy(), coreSpec, nil); err != nil {
		return nil, err
	}
	return coreSpec, nil
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}

func validateUnitedDeploymentSpecUpdate(spec, oldSpec *unitv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateWorkloadTemplateUpdate(&spec.WorkloadTemplate, &oldSpec.WorkloadTemplate, fldPath.Child("template"))...)
	allErrs = append(allErrs, validateUnitedDeploymentTopology(&spec.Topology, &oldSpec.Topology, fldPath.Child("topology"))...)
	return allErrs
}

func validateUnitedDeploymentTopology(topology, oldTopology *unitv1alpha1.Topology, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if topology == nil || oldTopology == nil {
		return allErrs
	}

	oldPools := map[string]*unitv1alpha1.Pool{}
	for i, pool := range oldTopology.Pools {
		oldPools[pool.Name] = &oldTopology.Pools[i]
	}

	for i, pool := range topology.Pools {
		if oldPool, exist := oldPools[pool.Name]; exist {
			if !apiequality.Semantic.DeepEqual(oldPool.NodeSelectorTerm, pool.NodeSelectorTerm) {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("pools").Index(i).Child("nodeSelectorTerm"), "may not be changed in an update"))
			}
			if !apiequality.Semantic.DeepEqual(oldPool.Tolerations, pool.Tolerations) {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("pools").Index(i).Child("tolerations"), "may not be changed in an update"))
			}
		}
	}
	return allErrs
}

func validateWorkloadTemplateUpdate(template, oldTemplate *unitv1alpha1.WorkloadTemplate, fldPath *field.Path) field.ErrorList {
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

func validatePoolTemplate(template *unitv1alpha1.WorkloadTemplate, spec *unitv1alpha1.UnitedDeploymentSpec,
	selector labels.Selector, fldPath *field.Path) field.ErrorList {

	allErrs := field.ErrorList{}

	var templateCount int
	if template.StatefulSetTemplate != nil {
		templateCount++
	}
	if template.DeploymentTemplate != nil {
		templateCount++
	}

	if templateCount < 1 {
		allErrs = append(allErrs, field.Required(fldPath, "should provide one of (statefulSetTemplate/deploymentTemplate/daemonSetTemplate)"))
	} else if templateCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, template, "should provide only one of (statefulSetTemplate/deploymentTemplate/daemonSetTemplate)"))
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
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("statefulSetTemplate", "spec", "template"))...)
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
			fldPath.Child("deploymentTemplate", "spec", "template"))...)
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

func validateStatefulSet(statefulSet *unitv1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *statefulSet.Spec.Replicas, "replicas in statefulSetTemplate will not be used"))
	}
	if statefulSet.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate != nil &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "updateStrategy", "rollingUpdate", "partition"), *statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition, "partition in statefulSetTemplate will not be used"))
	}

	return allErrs
}

func validateDeployment(deployment *unitv1alpha1.DeploymentTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if deployment.Spec.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *deployment.Spec.Replicas, "replicas in deploymentTemplate will not be used"))
	}
	return allErrs
}

func validateDeploymentUpdate(deployment, oldDeployment *unitv1alpha1.DeploymentTemplateSpec,
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

func validateStatefulSetUpdate(statefulSet, oldStatefulSet *unitv1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
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
