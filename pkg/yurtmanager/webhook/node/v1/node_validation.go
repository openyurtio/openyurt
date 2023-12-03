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

package v1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeHandler) ValidateCreate(_ context.Context, obj runtime.Object, req admission.Request) error {
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object, req admission.Request) error {
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Node but got a %T", newObj))
	}
	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Node} but got a %T", oldObj))
	}

	if allErrs := validateNodeUpdate(newNode, oldNode, req); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1.SchemeGroupVersion.WithKind("Node").GroupKind(), newNode.Name, allErrs)
	}

	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeHandler) ValidateDelete(_ context.Context, obj runtime.Object, req admission.Request) error {
	return nil
}

func validateNodeUpdate(newNode, oldNode *v1.Node, req admission.Request) field.ErrorList {
	oldNp := oldNode.Labels[projectinfo.GetNodePoolLabel()]
	newNp := newNode.Labels[projectinfo.GetNodePoolLabel()]
	oldNpHostNetwork := oldNode.Labels[apps.NodePoolHostNetworkLabel]
	newNpHostNetwork := newNode.Labels[apps.NodePoolHostNetworkLabel]

	var errList field.ErrorList
	// it is not allowed to change NodePoolLabel if it has been set
	if len(oldNp) != 0 && oldNp != newNp {
		errList = append(errList, field.Forbidden(field.NewPath("metadata").Child("labels").Child(projectinfo.GetNodePoolLabel()), "apps.openyurt.io/nodepool can not be changed"))
	}

	// it is not allowed to change NodePoolHostNetworkLabel if it has been set
	if len(oldNpHostNetwork) != 0 && oldNpHostNetwork != newNpHostNetwork {
		errList = append(errList, field.Forbidden(field.NewPath("metadata").Child("labels").Child(apps.NodePoolHostNetworkLabel), "nodepool.openyurt.io/hostnetwork can not be changed"))
	}

	if len(errList) != 0 {
		return errList
	}
	return nil
}
