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

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	unitv1beta2 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	util "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/utils"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateCreate(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	platformAdmin, ok := obj.(*v1beta1.PlatformAdmin)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", obj))
	}

	//validate
	if allErrs := webhook.validate(ctx, platformAdmin); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			v1beta1.GroupVersion.WithKind("PlatformAdmin").GroupKind(),
			platformAdmin.Name,
			allErrs,
		)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateUpdate(
	ctx context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	newPlatformAdmin, ok := newObj.(*v1beta1.PlatformAdmin)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", newObj))
	}
	oldPlatformAdmin, ok := oldObj.(*v1beta1.PlatformAdmin)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", oldObj))
	}

	// validate
	newErrorList := webhook.validate(ctx, newPlatformAdmin)
	oldErrorList := webhook.validate(ctx, oldPlatformAdmin)
	if allErrs := append(newErrorList, oldErrorList...); len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			v1beta1.GroupVersion.WithKind("PlatformAdmin").GroupKind(),
			newPlatformAdmin.Name,
			allErrs,
		)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *PlatformAdminHandler) validate(
	ctx context.Context,
	platformAdmin *v1beta1.PlatformAdmin,
) field.ErrorList {
	// verify the version
	if specErrs := webhook.validatePlatformAdminSpec(platformAdmin); specErrs != nil {
		return specErrs
	}

	// verify that the poolname nodepool
	if nodePoolErrs := webhook.validatePlatformAdminWithNodePools(ctx, platformAdmin); nodePoolErrs != nil {
		return nodePoolErrs
	}
	return nil
}

func (webhook *PlatformAdminHandler) validatePlatformAdminSpec(platformAdmin *v1beta1.PlatformAdmin) field.ErrorList {
	// TODO: Need to divert traffic based on the type of platform

	// Verify that the platform is supported
	if platformAdmin.Spec.Platform != v1beta1.PlatformAdminPlatformEdgeX {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "platform"),
				platformAdmin.Spec.Platform,
				"must be "+v1beta1.PlatformAdminPlatformEdgeX,
			),
		}
	}

	// Verify that it is a supported platformadmin version
	for _, version := range webhook.Manifests.Versions {
		if platformAdmin.Spec.Version == version.Name {
			return nil
		}
	}

	return field.ErrorList{
		field.Invalid(
			field.NewPath("spec", "version"),
			platformAdmin.Spec.Version,
			"must be one of"+strings.Join(config.ExtractVersionsName(webhook.Manifests).UnsortedList(), ","),
		),
	}
}

func (webhook *PlatformAdminHandler) validatePlatformAdminWithNodePools(
	ctx context.Context,
	platformAdmin *v1beta1.PlatformAdmin,
) field.ErrorList {
	// verify that the poolnames are right nodepool names
	nodepools := &unitv1beta2.NodePoolList{}
	if err := webhook.Client.List(ctx, nodepools); err != nil {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "nodepools"),
				platformAdmin.Spec.NodePools,
				"can not list nodepools, cause"+err.Error(),
			),
		}
	}

	nodePoolMap := make(map[string]bool)
	for _, nodePool := range nodepools.Items {
		nodePoolMap[nodePool.ObjectMeta.Name] = true
	}

	invalidPools := []string{}
	for _, poolName := range platformAdmin.Spec.NodePools {
		if !nodePoolMap[poolName] {
			invalidPools = append(invalidPools, poolName)
		}
	}
	if len(invalidPools) > 0 {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "nodepools"), invalidPools, "can not find the nodepools"),
		}
	}

	// verify that no other platformadmin in the nodepools
	var platformadmins v1beta1.PlatformAdminList
	if err := webhook.Client.List(ctx, &platformadmins); err != nil {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "nodepools"),
				platformAdmin.Spec.NodePools,
				"can not list platformadmins, cause"+err.Error(),
			),
		}
	}

	for _, other := range platformadmins.Items {
		if platformAdmin.Name != other.Name {
			for _, poolName := range platformAdmin.Spec.NodePools {
				if util.Contains(other.Spec.NodePools, poolName) {
					return field.ErrorList{
						field.Invalid(
							field.NewPath("spec", "nodepools"),
							poolName,
							"already used by other platformadmin instance",
						),
					}
				}
			}
		}
	}

	return nil
}
