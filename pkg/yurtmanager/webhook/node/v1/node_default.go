/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

// Default satisfies the defaulting webhook interface.
func (webhook *NodeHandler) Default(ctx context.Context, obj runtime.Object, req admission.Request) error {
	node, ok := obj.(*v1.Node)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Node but got a %T", obj))
	}

	npName, ok := node.Labels[apps.NodePoolLabel]
	if !ok {
		return nil
	}

	var np appsv1beta1.NodePool
	if err := webhook.Client.Get(ctx, types.NamespacedName{Name: npName}, &np); err != nil {
		return err
	}

	// add NodePool.Spec.Type to node labels
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels[apps.NodePoolTypeLabel] = strings.ToLower(string(np.Spec.Type))

	if np.Spec.HostNetwork {
		node.Labels[apps.NodePoolHostNetworkLabel] = "true"
	}
	return nil
}
