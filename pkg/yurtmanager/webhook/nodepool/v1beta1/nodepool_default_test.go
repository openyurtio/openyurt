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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

func TestDefault(t *testing.T) {
	testcases := map[string]struct {
		obj            runtime.Object
		errHappened    bool
		wantedNodePool *v1beta1.NodePool
	}{
		"it is not a nodepool": {
			obj:         &corev1.Pod{},
			errHappened: true,
		},
		"nodepool has no type": {
			obj: &v1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1beta1.NodePoolSpec{
					HostNetwork: true,
				},
			},
			wantedNodePool: &v1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1beta1.NodePoolSpec{
					HostNetwork: true,
					Type:        v1beta1.Edge,
				},
				Status: v1beta1.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
		"nodepool has pool type": {
			obj: &v1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1beta1.NodePoolSpec{
					HostNetwork: true,
					Type:        v1beta1.Cloud,
				},
			},
			wantedNodePool: &v1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v1beta1.NodePoolSpec{
					HostNetwork: true,
					Type:        v1beta1.Cloud,
				},
				Status: v1beta1.NodePoolStatus{
					ReadyNodeNum:   0,
					UnreadyNodeNum: 0,
					Nodes:          []string{},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := NodePoolHandler{}
			err := h.Default(context.TODO(), tc.obj)
			if tc.errHappened {
				if err == nil {
					t.Errorf("expect error, got nil")
				}
			} else if err != nil {
				t.Errorf("expect no error, but got %v", err)
			} else {
				currentNp := tc.obj.(*v1beta1.NodePool)
				if !reflect.DeepEqual(currentNp, tc.wantedNodePool) {
					t.Errorf("expect %#+v, got %#+v", tc.wantedNodePool, currentNp)
				}
			}
		})
	}
}
