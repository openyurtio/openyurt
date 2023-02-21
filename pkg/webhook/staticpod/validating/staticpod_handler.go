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

package validating

import (
	"context"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// StaticPodCreateUpdateHandler handles StaticPod
type StaticPodCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

const (
	webhookName = "StaticPod-validate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

var _ admission.Handler = &StaticPodCreateUpdateHandler{}

// Handle handles admission requests.
func (h *StaticPodCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle StaticPod %s/%s", req.Namespace, req.Name))

	obj := &appsv1alpha1.StaticPod{}

	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.Warningf("Error validate StaticPod %s: %v", obj.Name, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1alpha1.StaticPod) error {
	if allErrs := validateStaticPodSpec(&obj.Spec); len(allErrs) > 0 {
		return apierrors.NewInvalid(appsv1alpha1.GroupVersion.WithKind("StaticPod").GroupKind(), obj.Name, allErrs)
	}

	klog.Infof(Format("Validate StaticPod %s successfully ...", klog.KObj(obj)))

	return nil
}

var _ admission.DecoderInjector = &StaticPodCreateUpdateHandler{}

// InjectDecoder injects the decoder into the StaticPodCreateUpdateHandler
func (h *StaticPodCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// validateStaticPodSpec validates the staticpod spec.
func validateStaticPodSpec(spec *appsv1alpha1.StaticPodSpec) field.ErrorList {
	var allErrs field.ErrorList

	if spec.StaticPodName == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("StaticPodName"),
			"StaticPodName is required"))
	}

	if spec.StaticPodManifest == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("StaticPodManifest"),
			"StaticPodManifest is required"))
	}

	if spec.StaticPodNamespace == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("Namespace"),
			"Namespace is required"))
	}

	strategy := &spec.UpgradeStrategy
	if strategy == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("upgradeStrategy"),
			"upgrade strategy is required"))
	}

	if strategy.Type != appsv1alpha1.AutoStaticPodUpgradeStrategyType && strategy.Type != appsv1alpha1.OTAStaticPodUpgradeStrategyType {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("spec").Child("upgradeStrategy"),
			strategy, []string{"auto", "ota"}))
	}

	if strategy.Type == appsv1alpha1.AutoStaticPodUpgradeStrategyType && strategy.MaxUnavailable == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("upgradeStrategy"),
			"max-unavailable is required in auto mode"))
	}

	if allErrs != nil {
		return allErrs
	}

	return nil
}
