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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestYurtAppDaemonHandler_Default(t *testing.T) {
	tests := []struct {
		name           string
		obj            runtime.Object
		expectedErr    error
		validateResult func(t *testing.T, daemon *v1alpha1.YurtAppDaemon)
	}{
		{
			name: "valid YurtAppDaemon with StatefulSetTemplate",
			obj: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-daemon",
					Namespace: "default",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "yurt-daemon"},
					},
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						StatefulSetTemplate: &v1alpha1.StatefulSetTemplateSpec{},
					},
				},
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, daemon *v1alpha1.YurtAppDaemon) {
				assert.NotNil(t, daemon.Status)
				assert.NotNil(t, daemon.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Selector)
				assert.Equal(t, daemon.Spec.Selector, daemon.Spec.WorkloadTemplate.StatefulSetTemplate.Spec.Selector)
			},
		},
		{
			name: "valid YurtAppDaemon with DeploymentTemplate",
			obj: &v1alpha1.YurtAppDaemon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-daemon",
					Namespace: "default",
				},
				Spec: v1alpha1.YurtAppDaemonSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "yurt-daemon"},
					},
					WorkloadTemplate: v1alpha1.WorkloadTemplate{
						DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{},
					},
				},
			},
			expectedErr: nil,
			validateResult: func(t *testing.T, daemon *v1alpha1.YurtAppDaemon) {
				assert.NotNil(t, daemon.Status)
				assert.NotNil(t, daemon.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Selector)
				assert.Equal(t, daemon.Spec.Selector, daemon.Spec.WorkloadTemplate.DeploymentTemplate.Spec.Selector)
			},
		},
		{
			name:        "invalid object type",
			obj:         &v1alpha1.YurtAppSet{}, // wrong object type
			expectedErr: apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppDaemon but got a %T", &v1alpha1.YurtAppSet{})),
			validateResult: func(t *testing.T, daemon *v1alpha1.YurtAppDaemon) {
				assert.Nil(t, daemon)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &YurtAppDaemonHandler{}
			err := handler.Default(context.Background(), tt.obj)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				daemon, ok := tt.obj.(*v1alpha1.YurtAppDaemon)
				if ok && tt.validateResult != nil {
					tt.validateResult(t, daemon)
				}
			}
		})
	}
}
