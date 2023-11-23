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
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestValidateUpdate(t *testing.T) {
	testcases := map[string]struct {
		oldNode runtime.Object
		newNode runtime.Object
		errCode int
	}{
		"old object is not a node": {
			oldNode: &corev1.Pod{},
			newNode: &corev1.Node{},
			errCode: http.StatusBadRequest,
		},
		"new object is not a node": {
			oldNode: &corev1.Node{},
			newNode: &corev1.Pod{},
			errCode: http.StatusBadRequest,
		},
		"node pool is changed": {
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "shanghai",
					},
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		"node pool host network is changed": {
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
						apps.NodePoolHostNetworkLabel:  "true",
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
						apps.NodePoolHostNetworkLabel:  "false",
					},
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		"it is a normal node update": {
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
						apps.NodePoolHostNetworkLabel:  "true",
					},
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
						apps.NodePoolHostNetworkLabel:  "true",
					},
				},
			},
			errCode: 0,
		},
		"it is a normal node update without init labels": {
			oldNode: &corev1.Node{},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
						apps.NodePoolHostNetworkLabel:  "true",
					},
				},
			},
			errCode: 0,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := &NodeHandler{}
			err := h.ValidateUpdate(context.TODO(), tc.oldNode, tc.newNode, admission.Request{})
			if tc.errCode == 0 && err != nil {
				t.Errorf("Expected error code %d, got %v", tc.errCode, err)
			} else if tc.errCode != 0 {
				statusErr := err.(*errors.StatusError)
				if tc.errCode != int(statusErr.Status().Code) {
					t.Errorf("Expected error code %d, got %v", tc.errCode, err)
				}
			}
		})
	}
}
