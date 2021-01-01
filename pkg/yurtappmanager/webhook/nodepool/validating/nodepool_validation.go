/*
Copyright 2020 The OpenYurt Authors.

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
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/alibaba/openyurt/pkg/yurtappmanager/apis/apps/v1alpha1"
)

// annotationValidator validates the NodePool.Spec.Annotations
var annotationValidator = func(annos map[string]string) error {
	errs := apivalidation.ValidateAnnotations(annos, field.NewPath("field"))
	if len(errs) > 0 {
		return errors.New(errs.ToAggregate().Error())
	}
	return nil
}

func validateNodePoolSpecAnnotations(annotations map[string]string) field.ErrorList {
	if err := annotationValidator(annotations); err != nil {
		return field.ErrorList([]*field.Error{
			field.Invalid(field.NewPath("spec").Child("annotations"),
				annotations, "invalid annotations")})
	}
	return nil
}

// validateNodePoolSpec validates the nodepool spec.
func validateNodePoolSpec(spec *appsv1alpha1.NodePoolSpec) field.ErrorList {
	if allErrs := validateNodePoolSpecAnnotations(spec.Annotations); allErrs != nil {
		return allErrs
	}
	return nil
}

// validateNodePoolSpecUpdate tests if required fields in the NodePool spec are set.
func validateNodePoolSpecUpdate(spec, oldSpec *appsv1alpha1.NodePoolSpec) field.ErrorList {
	if allErrs := validateNodePoolSpec(spec); allErrs != nil {
		return allErrs
	}

	if spec.Type != oldSpec.Type {
		return field.ErrorList([]*field.Error{
			field.Invalid(field.NewPath("spec").Child("type"),
				spec.Annotations, "pool type can't be changed")})
	}
	return nil
}

// validateNodePoolDeletion validate the nodepool deletion event, which prevents
// the default-nodepool from being deleted
func validateNodePoolDeletion(cli client.Client, np *appsv1alpha1.NodePool) field.ErrorList {
	nodes := corev1.NodeList{}

	if np.Name == appsv1alpha1.DefaultCloudNodePoolName || np.Name == appsv1alpha1.DefaultEdgeNodePoolName {
		return field.ErrorList([]*field.Error{
			field.Forbidden(field.NewPath("metadata").Child("name"),
				fmt.Sprintf("default nodepool %s forbiden to delete", np.Name))})
	}

	if err := cli.List(context.TODO(), &nodes,
		client.MatchingLabels(np.Spec.Selector.MatchLabels)); err != nil {
		return field.ErrorList([]*field.Error{
			field.Forbidden(field.NewPath("metadata").Child("name"),
				"fail to get nodes associated to the pool")})
	}
	if len(nodes.Items) != 0 {
		return field.ErrorList([]*field.Error{
			field.Forbidden(field.NewPath("metadata").Child("name"),
				"cannot remove nonempty pool, please drain the pool before deleting")})
	}
	return nil
}
