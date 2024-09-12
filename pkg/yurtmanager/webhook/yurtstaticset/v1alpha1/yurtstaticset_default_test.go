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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestYurtStaticSetHandler_Default(t *testing.T) {
	tests := []struct {
		name        string
		obj         runtime.Object
		expectedErr error
	}{
		{
			name: "valid YurtStaticSet",
			obj: &v1alpha1.YurtStaticSet{
				Spec: v1alpha1.YurtStaticSetSpec{},
			},
			expectedErr: nil,
		},
		{
			name:        "invalid object type",
			obj:         &v1alpha1.YurtAppSet{},
			expectedErr: apierrors.NewBadRequest(fmt.Sprintf("expected a YurtStaticSet but got a %T", &v1alpha1.YurtAppSet{})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &YurtStaticSetHandler{}
			err := handler.Default(context.Background(), tt.obj)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				_, ok := tt.obj.(*v1alpha1.YurtStaticSet)
				assert.True(t, ok, "Expected object to be of type YurtStaticSet")
			}
		})
	}
}
