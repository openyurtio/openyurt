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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/raven"
	"github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
)

// TestDefault tests the Default method of GatewayHandler.
func TestDefault(t *testing.T) {
	type testCase struct {
		name         string
		obj          runtime.Object
		expected     error
		nodeSelector *metav1.LabelSelector
	}

	tests := []testCase{
		{
			name:     "should get StatusError when invalid PlatformAdmin type",
			obj:      &unstructured.Unstructured{},
			expected: apierrors.NewBadRequest(fmt.Sprintf("expected a Gateway but got a %T", &unstructured.Unstructured{})),
		},
		{
			name: "should get no error when valid PlatformAdmin object with spec version is v1",
			obj: &v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gateway",
				},
			},
			nodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					raven.LabelCurrentGateway: "test-gateway",
				},
			},
			expected: nil,
		},
	}

	handler := &GatewayHandler{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := handler.Default(context.TODO(), tc.obj)
			assert.Equal(t, tc.expected, err)
			if err == nil {
				assert.Equal(t, tc.nodeSelector, tc.obj.(*v1alpha1.Gateway).Spec.NodeSelector)
			}
		})
	}
}
