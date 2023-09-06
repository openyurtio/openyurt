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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppOverriderHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	overrider, ok := obj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", obj))
	}

	// validate
	if err := webhook.validateOneToOneBinding(ctx, overrider); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppOverriderHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	_, ok := oldObj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", newObj))
	}
	newOverrider, ok := newObj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider} but got a %T", oldObj))
	}

	// validate
	if err := webhook.validateOneToOneBinding(ctx, newOverrider); err != nil {
		return err
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppOverriderHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	return nil
}

// YurtAppOverrider and YurtAppSet are one-to-one relationship
func (webhook *YurtAppOverriderHandler) validateOneToOneBinding(ctx context.Context, yurtAppOverrider *v1alpha1.YurtAppOverrider) error {
	app := yurtAppOverrider
	var allOverriderList v1alpha1.YurtAppOverriderList
	if err := webhook.Client.List(ctx, &allOverriderList, client.InNamespace(yurtAppOverrider.Namespace)); err != nil {
		klog.Infof("could not list YurtAppOverrider, %v", err)
		return err
	}
	overriderList := make([]v1alpha1.YurtAppOverrider, 0)
	for _, overrider := range allOverriderList.Items {
		if overrider.Name == app.Name && overrider.Kind == app.Kind {
			continue
		}
		if overrider.Subject.Kind == app.Subject.Kind && overrider.Subject.Name == app.Subject.Name {
			overriderList = append(overriderList, overrider)
		}
	}
	if len(overriderList) > 0 {
		return fmt.Errorf("only one YurtAppOverrider can be bound into one YurtAppSet")
	}
	return nil
}
