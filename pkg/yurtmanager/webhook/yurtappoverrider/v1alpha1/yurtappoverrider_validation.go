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
	oldOverrider, ok := oldObj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", newObj))
	}
	newOverrider, ok := newObj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider} but got a %T", oldObj))
	}
	if oldOverrider.Namespace != newOverrider.Namespace || newOverrider.Name != oldOverrider.Name {
		return fmt.Errorf("unable to change metadata after %s is created", oldOverrider.Name)
	}
	if newOverrider.Subject.Kind != oldOverrider.Subject.Kind || newOverrider.Subject.Name != oldOverrider.Subject.Name {
		return fmt.Errorf("unable to modify subject after %s is created", oldOverrider.Name)
	}
	// validate
	if err := webhook.validateOneToOneBinding(ctx, newOverrider); err != nil {
		return err
	}
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppOverriderHandler) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	overrider, ok := obj.(*v1alpha1.YurtAppOverrider)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", obj))
	}
	switch overrider.Subject.Kind {
	case "YurtAppSet":
		appSet := &v1alpha1.YurtAppSet{}
		err := webhook.Client.Get(ctx, client.ObjectKey{Name: overrider.Subject.Name, Namespace: overrider.Namespace}, appSet)
		if err == nil {
			return fmt.Errorf("namespace: %s, unable to delete YurtAppOverrider when subject resource exists: %s", overrider.Namespace, appSet.Name)
		}
	case "YurtAppDaemon":
		appDaemon := &v1alpha1.YurtAppDaemon{}
		err := webhook.Client.Get(ctx, client.ObjectKey{Name: overrider.Subject.Name, Namespace: overrider.Namespace}, appDaemon)
		if err == nil {
			return fmt.Errorf("namespace: %s, unable to delete YurtAppOverrider when subject resource exists: %s", overrider.Namespace, appDaemon.Name)
		}
	}
	return nil
}

// YurtAppOverrider and YurtAppSet are one-to-one relationship
func (webhook *YurtAppOverriderHandler) validateOneToOneBinding(ctx context.Context, app *v1alpha1.YurtAppOverrider) error {
	var allOverriderList v1alpha1.YurtAppOverriderList
	if err := webhook.Client.List(ctx, &allOverriderList, client.InNamespace(app.Namespace)); err != nil {
		klog.Infof("could not list YurtAppOverrider, %v", err)
		return err
	}
	duplicatedOverriders := make([]v1alpha1.YurtAppOverrider, 0)
	for _, overrider := range allOverriderList.Items {
		if overrider.Name == app.Name {
			continue
		}
		if overrider.Subject.Kind == app.Subject.Kind && overrider.Subject.Name == app.Subject.Name {
			duplicatedOverriders = append(duplicatedOverriders, overrider)
		}
	}
	if len(duplicatedOverriders) > 0 {
		return fmt.Errorf("unable to bind multiple yurtappoverriders to one subject resource %s in namespace %s, %s already exists", app.Subject.Name, app.Namespace, app.Name)
	}
	return nil
}
