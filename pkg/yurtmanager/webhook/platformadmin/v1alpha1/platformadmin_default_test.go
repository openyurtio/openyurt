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

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
)

// TestDefault tests the Default method of PlatformAdminHandler.
func TestDefault(t *testing.T) {
	type testCase struct {
		name     string
		obj      runtime.Object
		expected error
		version  string
	}

	tests := []testCase{
		{
			name: "should get no error when valid PlatformAdmin object with spec empty version",
			obj: &v1alpha1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha1.PlatformAdminSpec{
					Version: "",
				},
			},
			expected: nil,
			version:  "",
		},
		{
			name: "should get no error when valid PlatformAdmin object with spec version is v1",
			obj: &v1alpha1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha1.PlatformAdminSpec{
					Version: "v1",
				},
			},
			expected: nil,
			version:  "v1",
		},
		{
			name:     "should get StatusError when invalid PlatformAdmin type",
			obj:      &unstructured.Unstructured{},
			expected: apierrors.NewBadRequest(fmt.Sprintf("expected a PlatformAdmin but got a %T", &unstructured.Unstructured{})),
		},
	}

	handler := PlatformAdminHandler{Manifests: &config.Manifest{}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := handler.Default(context.TODO(), tc.obj)
			assert.Equal(t, tc.expected, err)
			if err == nil {
				assert.Equal(t, tc.version, tc.obj.(*v1alpha1.PlatformAdmin).Spec.Version)
			}
		})
	}
}
