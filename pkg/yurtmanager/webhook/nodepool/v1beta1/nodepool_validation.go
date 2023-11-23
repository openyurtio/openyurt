/*
Copyright 2023 The OpenYurt Authors.

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

package v1beta1

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodePoolHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	np, ok := obj.(*appsv1beta1.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", obj))
	}

	if allErrs := validateNodePoolSpec(&np.Spec); len(allErrs) > 0 {
		return apierrors.NewInvalid(appsv1beta1.GroupVersion.WithKind("NodePool").GroupKind(), np.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodePoolHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newNp, ok := newObj.(*appsv1beta1.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", newObj))
	}
	oldNp, ok := oldObj.(*appsv1beta1.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", oldObj))
	}

	if allErrs := validateNodePoolSpecUpdate(&newNp.Spec, &oldNp.Spec); len(allErrs) > 0 {
		return apierrors.NewInvalid(appsv1beta1.GroupVersion.WithKind("NodePool").GroupKind(), newNp.Name, allErrs)
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodePoolHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	np, ok := obj.(*appsv1beta1.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", obj))
	}
	if allErrs := validateNodePoolDeletion(webhook.Client, np); len(allErrs) > 0 {
		return apierrors.NewInvalid(appsv1beta1.GroupVersion.WithKind("NodePool").GroupKind(), np.Name, allErrs)
	}

	return nil
}

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
func validateNodePoolSpec(spec *appsv1beta1.NodePoolSpec) field.ErrorList {
	if allErrs := validateNodePoolSpecAnnotations(spec.Annotations); allErrs != nil {
		return allErrs
	}

	// NodePool type should be Edge or Cloud
	if spec.Type != appsv1beta1.Edge && spec.Type != appsv1beta1.Cloud {
		return []*field.Error{field.Invalid(field.NewPath("spec").Child("type"), spec.Type, "pool type should be Edge or Cloud")}
	}

	// Cloud NodePool can not set HostNetwork=true
	if spec.Type == appsv1beta1.Cloud && spec.HostNetwork {
		return []*field.Error{field.Invalid(field.NewPath("spec").Child("hostNetwork"), spec.HostNetwork, "Cloud NodePool cloud not support hostNetwork")}
	}
	return nil
}

// validateNodePoolSpecUpdate tests if required fields in the NodePool spec are set.
func validateNodePoolSpecUpdate(spec, oldSpec *appsv1beta1.NodePoolSpec) field.ErrorList {
	if allErrs := validateNodePoolSpec(spec); allErrs != nil {
		return allErrs
	}

	if spec.Type != oldSpec.Type {
		return field.ErrorList([]*field.Error{
			field.Invalid(field.NewPath("spec").Child("type"), spec.Type, "pool type can't be changed")})
	}

	if spec.HostNetwork != oldSpec.HostNetwork {
		return field.ErrorList([]*field.Error{
			field.Invalid(field.NewPath("spec").Child("hostNetwork"), spec.HostNetwork, "pool hostNetwork can't be changed"),
		})
	}
	return nil
}

// validateNodePoolDeletion validate the nodepool deletion event, which prevents
// the default-nodepool from being deleted
func validateNodePoolDeletion(cli client.Client, np *appsv1beta1.NodePool) field.ErrorList {
	nodes := corev1.NodeList{}

	if err := cli.List(context.TODO(), &nodes, client.MatchingLabels(map[string]string{projectinfo.GetNodePoolLabel(): np.Name})); err != nil {
		return field.ErrorList([]*field.Error{
			field.Forbidden(field.NewPath("metadata").Child("name"),
				"could not get nodes associated to the pool")})
	}
	if len(nodes.Items) != 0 {
		return field.ErrorList([]*field.Error{
			field.Forbidden(field.NewPath("metadata").Child("name"),
				"cannot remove nonempty pool, please drain the pool before deleting")})
	}
	return nil
}
