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

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// Default satisfies the defaulting webhook interface.
func (webhook *YurtAppDaemonHandler) Default(ctx context.Context, obj runtime.Object) error {
	klog.Info("default object %v", obj)
	daemon, ok := obj.(*v1alpha1.YurtAppDaemon)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", obj))
	}

	v1alpha1.SetDefaultsYurtAppDaemon(daemon)
	daemon.Status = v1alpha1.YurtAppDaemonStatus{}

	statefulSetTemp := daemon.Spec.WorkloadTemplate.StatefulSetTemplate
	deployTem := daemon.Spec.WorkloadTemplate.DeploymentTemplate

	if statefulSetTemp != nil {
		statefulSetTemp.Spec.Selector = daemon.Spec.Selector
	}
	if deployTem != nil {
		deployTem.Spec.Selector = daemon.Spec.Selector
	}

	return nil
}
