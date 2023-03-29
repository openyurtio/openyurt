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

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/webhook/util"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// SetupWebhookWithManager sets up Cluster webhooks.
func (webhook *YurtAppDaemonHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {

	gvk, err := apiutil.GVKForObject(&appsv1alpha1.YurtAppDaemon{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}
	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		ctrl.NewWebhookManagedBy(mgr).
			For(&v1alpha1.YurtAppDaemon{}).
			WithDefaulter(webhook).
			WithValidator(webhook).
			Complete()
}

// +kubebuilder:webhook:path=/validate-apps-openyurt-io-yurtappdaemon,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps.openyurt.io,resources=yurtappdaemons,verbs=create;update,versions=v1alpha1,name=validate.apps.v1alpha1.yurtappdaemon.openyurt.io
// +kubebuilder:webhook:path=/mutate-apps-openyurt-io-yurtappdaemon,mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps.openyurt.io,resources=yurtappdaemons,verbs=create;update,versions=v1alpha1,name=mutate.apps.v1alpha1.yurtappdaemon.openyurt.io

// Cluster implements a validating and defaulting webhook for Cluster.
type YurtAppDaemonHandler struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &YurtAppDaemonHandler{}
var _ webhook.CustomValidator = &YurtAppDaemonHandler{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppDaemonHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	daemon, ok := obj.(*v1alpha1.YurtAppDaemon)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", obj))
	}

	if allErrs := validateYurtAppDaemon(webhook.Client, daemon); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("YurtAppDaemon").GroupKind(), daemon.Name, allErrs)
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
		return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("YurtAppDaemon").GroupKind(), newDaemon.Name, allErrs)
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppDaemonHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}
