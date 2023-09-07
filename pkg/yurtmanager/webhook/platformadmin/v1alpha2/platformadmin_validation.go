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

package v1alpha2

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	util "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/utils"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	platformAdmin, ok := obj.(*v1alpha2.PlatformAdmin)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", obj))
	}

	//validate
	if allErrs := webhook.validate(ctx, platformAdmin); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha2.GroupVersion.WithKind("PlatformAdmin").GroupKind(), platformAdmin.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newPlatformAdmin, ok := newObj.(*v1alpha2.PlatformAdmin)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", newObj))
	}
	oldPlatformAdmin, ok := oldObj.(*v1alpha2.PlatformAdmin)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", oldObj))
	}

	// validate
	newErrorList := webhook.validate(ctx, newPlatformAdmin)
	oldErrorList := webhook.validate(ctx, oldPlatformAdmin)
	if allErrs := append(newErrorList, oldErrorList...); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha2.GroupVersion.WithKind("PlatformAdmin").GroupKind(), newPlatformAdmin.Name, allErrs)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PlatformAdminHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func (webhook *PlatformAdminHandler) validate(ctx context.Context, platformAdmin *v1alpha2.PlatformAdmin) field.ErrorList {

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

func (webhook *PlatformAdminHandler) validatePlatformAdminSpec(platformAdmin *v1alpha2.PlatformAdmin) field.ErrorList {
	// TODO: Need to divert traffic based on the type of platform

	// Verify that the platform is supported
	if platformAdmin.Spec.Platform != v1alpha2.PlatformAdminPlatformEdgeX {
		return field.ErrorList{field.Invalid(field.NewPath("spec", "platform"), platformAdmin.Spec.Platform, "must be "+v1alpha2.PlatformAdminPlatformEdgeX)}
	}

	// Verify that it is a supported platformadmin version
	for _, version := range webhook.Manifests.Versions {
		if platformAdmin.Spec.Version == version.Name {
			return nil
		}
	}

	return field.ErrorList{
		field.Invalid(field.NewPath("spec", "version"), platformAdmin.Spec.Version, "must be one of"+strings.Join(config.ExtractVersionsName(webhook.Manifests).List(), ",")),
	}
}

func (webhook *PlatformAdminHandler) validatePlatformAdminWithNodePools(ctx context.Context, platformAdmin *v1alpha2.PlatformAdmin) field.ErrorList {
	// verify that the poolname is a right nodepool name
	nodePools := &unitv1alpha1.NodePoolList{}
	if err := webhook.Client.List(ctx, nodePools); err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), platformAdmin.Spec.PoolName, "can not list nodepools, cause"+err.Error()),
		}
	}
	ok := false
	for _, nodePool := range nodePools.Items {
		if nodePool.ObjectMeta.Name == platformAdmin.Spec.PoolName {
			ok = true
			break
		}
	}
	if !ok {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), platformAdmin.Spec.PoolName, "can not find the nodepool"),
		}
	}
	// verify that no other platformadmin in the nodepool
	var platformadmins v1alpha2.PlatformAdminList
	listOptions := client.MatchingFields{util.IndexerPathForNodepool: platformAdmin.Spec.PoolName}
	if err := webhook.Client.List(ctx, &platformadmins, listOptions); err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec", "poolName"), platformAdmin.Spec.PoolName, "can not list platformadmins, cause "+err.Error()),
		}
	}
	for _, other := range platformadmins.Items {
		if platformAdmin.Name != other.Name {
			return field.ErrorList{
				field.Invalid(field.NewPath("spec", "poolName"), platformAdmin.Spec.PoolName, "already used by other platformadmin instance,"),
			}
		}
	}

	return nil
}
