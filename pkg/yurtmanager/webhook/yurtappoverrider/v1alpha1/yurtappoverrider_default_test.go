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
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestGatewayHandler_Default(t *testing.T) {
	tests := []struct {
		name     string
		obj      runtime.Object
		expected error
	}{
		{
			name:     "should return error when input is not YurtAppOverrider",
			obj:      &unstructured.Unstructured{},
			expected: apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppOverrider but got a %T", &unstructured.Unstructured{})),
		},
		{
			name:     "should set defaults when input is valid YurtAppOverrider",
			obj:      &v1alpha1.YurtAppOverrider{},
			expected: nil,
		},
	}

	webhook := &YurtAppOverriderHandler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := webhook.Default(context.TODO(), tt.obj)

			if tt.expected != nil {
				assert.Equal(t, tt.expected, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
