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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/core"
	k8s_api_v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	k8s_validation "k8s.io/kubernetes/pkg/apis/core/validation"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

const (
	YurtStaticSetKind = "YurtStaticSet"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtStaticSetHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	sp, ok := obj.(*v1alpha1.YurtStaticSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtStaticSet but got a %T", obj))
	}

	return validate(sp)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtStaticSetHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newSP, ok := newObj.(*v1alpha1.YurtStaticSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtStaticSet but got a %T", newObj))
	}
	oldSP, ok := oldObj.(*v1alpha1.YurtStaticSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtStaticSet but got a %T", oldObj))
	}

	if err := validate(newSP); err != nil {
		return err
	}

	if err := validate(oldSP); err != nil {
		return err
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtStaticSetHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func validate(obj *v1alpha1.YurtStaticSet) error {
	var allErrs field.ErrorList
	outPodTemplateSpec := &core.PodTemplateSpec{}
	if err := k8s_api_v1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(&obj.Spec.Template, outPodTemplateSpec, nil); err != nil {
		allErrs = append(allErrs, field.Required(field.NewPath("template"),
			"template filed should be corev1.PodTemplateSpec type"))
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(YurtStaticSetKind).GroupKind(), obj.Name, allErrs)
	}

	if e := k8s_validation.ValidatePodTemplateSpec(outPodTemplateSpec, field.NewPath("template"),
		k8s_validation.PodValidationOptions{}); len(e) > 0 {
		allErrs = append(allErrs, e...)
	}

	if e := validateYurtStaticSetSpec(&obj.Spec); len(e) > 0 {
		allErrs = append(allErrs, e...)
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(YurtStaticSetKind).GroupKind(), obj.Name, allErrs)
	}

	klog.Infof("Validate YurtStaticSet %s successfully ...", klog.KObj(obj))

	return nil
}

// validateYurtStaticSetSpec validates the YurtStaticSet spec.
func validateYurtStaticSetSpec(spec *v1alpha1.YurtStaticSetSpec) field.ErrorList {
	var allErrs field.ErrorList

	if spec.StaticPodManifest == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("StaticPodManifest"),
			"StaticPodManifest is required"))
	}

	strategy := &spec.UpgradeStrategy

	if !strings.EqualFold(string(strategy.Type), string(v1alpha1.OTAUpgradeStrategyType)) && !strings.EqualFold(string(strategy.Type), string(v1alpha1.AdvancedRollingUpdateUpgradeStrategyType)) {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("spec").Child("upgradeStrategy"),
			strategy, []string{"OTA", "AdvancedRollingUpdate"}))
	}

	if strings.EqualFold(string(strategy.Type), string(v1alpha1.AdvancedRollingUpdateUpgradeStrategyType)) && strategy.MaxUnavailable == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("upgradeStrategy"),
			"max-unavailable is required in AdvancedRollingUpdate mode"))
	}

	if allErrs != nil {
		return allErrs
	}

	return nil
}
