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

package v1beta1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

// Default satisfies the defaulting webhook interface.
func (webhook *NodePoolHandler) Default(ctx context.Context, obj runtime.Object) error {
	np, ok := obj.(*v1beta1.NodePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodePool but got a %T", obj))
	}

	// specify default type as Edge
	if len(np.Spec.Type) == 0 {
		np.Spec.Type = v1beta1.Edge
	}

	// init node pool status
	np.Status = v1beta1.NodePoolStatus{
		ReadyNodeNum:   0,
		UnreadyNodeNum: 0,
		Nodes:          make([]string, 0),
	}

	return nil
}
