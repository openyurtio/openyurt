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

package v1beta1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func TestYurtAppSetHandler_Default(t *testing.T) {
	handler := &YurtAppSetHandler{}

	tests := []struct {
		name     string
		obj      runtime.Object
		expected *int32
		wantErr  bool
		errMsg   string
	}{
		{
			name: "should set RevisionHistoryLimit to 10 when it is nil",
			obj: &v1beta1.YurtAppSet{
				Spec: v1beta1.YurtAppSetSpec{
					RevisionHistoryLimit: nil,
				},
			},
			expected: pointer.Int32(10),
			wantErr:  false,
		},
		{
			name: "should not change RevisionHistoryLimit when it is already set",
			obj: &v1beta1.YurtAppSet{
				Spec: v1beta1.YurtAppSetSpec{
					RevisionHistoryLimit: pointer.Int32(5),
				},
			},
			expected: pointer.Int32(5),
			wantErr:  false,
		},
		{
			name:    "should return an error when the object is not a YurtAppSet",
			obj:     &runtime.Unknown{},
			wantErr: true,
			errMsg:  "expected a YurtAppSet but got a *runtime.Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Default(context.Background(), tt.obj)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				appSet, ok := tt.obj.(*v1beta1.YurtAppSet)
				if !ok {
					t.Errorf("expected object to be of type YurtAppSet")
				} else if appSet.Spec.RevisionHistoryLimit == nil || *appSet.Spec.RevisionHistoryLimit != *tt.expected {
					t.Errorf("expected RevisionHistoryLimit to be %v, got %v", *tt.expected, appSet.Spec.RevisionHistoryLimit)
				}
			}
		})
	}
}
