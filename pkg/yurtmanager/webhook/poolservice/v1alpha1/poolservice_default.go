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

	"github.com/openyurtio/openyurt/pkg/apis/net/v1alpha1"
)

// Default satisfies the defaulting webhook interface.
func (webhook *PoolServiceHandler) Default(ctx context.Context, obj runtime.Object) error {
	ps, ok := obj.(*v1alpha1.PoolService)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a PoolService but got a %T", obj))
	}

	v1alpha1.SetDefaultsPoolService(ps)

	return nil
}
