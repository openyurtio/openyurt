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
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *StaticPodHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	sp, ok := obj.(*v1alpha1.StaticPod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a StaticPod but got a %T", obj))
	}

	return validate(sp)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *StaticPodHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newSP, ok := newObj.(*v1alpha1.StaticPod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a StaticPod but got a %T", newObj))
	}
	oldSP, ok := oldObj.(*v1alpha1.StaticPod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a StaticPod but got a %T", oldObj))
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
func (webhook *StaticPodHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

func validate(obj *v1alpha1.StaticPod) error {
	if allErrs := validateStaticPodSpec(&obj.Spec); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("StaticPod").GroupKind(), obj.Name, allErrs)
	}

	klog.Infof("Validate StaticPod %s successfully ...", klog.KObj(obj))

	return nil
}

// validateStaticPodSpec validates the staticpod spec.
func validateStaticPodSpec(spec *v1alpha1.StaticPodSpec) field.ErrorList {
	var allErrs field.ErrorList

	if spec.StaticPodManifest == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("StaticPodManifest"),
			"StaticPodManifest is required"))
	}

	strategy := &spec.UpgradeStrategy

	if strategy.Type != v1alpha1.AutoStaticPodUpgradeStrategyType && strategy.Type != v1alpha1.OTAStaticPodUpgradeStrategyType {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("spec").Child("upgradeStrategy"),
			strategy, []string{"auto", "ota"}))
	}

	if strategy.Type == v1alpha1.AutoStaticPodUpgradeStrategyType && strategy.MaxUnavailable == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("upgradeStrategy"),
			"max-unavailable is required in auto mode"))
	}

	if allErrs != nil {
		return allErrs
	}

	return nil
}
