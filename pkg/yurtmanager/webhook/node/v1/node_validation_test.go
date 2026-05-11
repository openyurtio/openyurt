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

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	webhookutil "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
)

func TestValidateUpdate(t *testing.T) {
	webhookutil.SetNamespace("kube-system")

	testcases := map[string]struct {
		oldNode runtime.Object
		newNode runtime.Object
		ctx     context.Context
		errCode int
	}{
		"old object is not a node": {
			oldNode: &corev1.Pod{},
			newNode: &corev1.Node{},
			ctx:     context.TODO(),
			errCode: http.StatusBadRequest,
		},
		"new object is not a node": {
			oldNode: &corev1.Node{},
			newNode: &corev1.Pod{},
			ctx:     context.TODO(),
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
			ctx:     context.TODO(),
			errCode: http.StatusUnprocessableEntity,
		},
		"node pool is removed": {
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
					},
				},
			},
			newNode: &corev1.Node{},
			ctx:     context.TODO(),
			errCode: 0,
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
			ctx:     context.TODO(),
			errCode: http.StatusUnprocessableEntity,
		},
		"is-edge-worker changed by unauthorized actor": {
			oldNode: &corev1.Node{},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			ctx:     newAdmissionContext("system:serviceaccount:default:random"),
			errCode: http.StatusUnprocessableEntity,
		},
		"is-edge-worker changed by conversion controller": {
			oldNode: &corev1.Node{},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			ctx:     newAdmissionContext("system:serviceaccount:kube-system:yurt-manager-" + names.YurtNodeConversionController),
			errCode: 0,
		},
		"is-edge-worker changed by node certificate process": {
			oldNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
				},
			},
			newNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-a",
					Labels: map[string]string{
						projectinfo.GetEdgeWorkerLabelKey(): "true",
					},
				},
			},
			ctx:     newAdmissionContext("system:node:node-a"),
			errCode: 0,
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
			ctx:     context.TODO(),
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
			ctx:     context.TODO(),
			errCode: 0,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := &NodeHandler{}
			_, err := h.ValidateUpdate(tc.ctx, tc.oldNode, tc.newNode)
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

func newAdmissionContext(username string) context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authv1.UserInfo{
				Username: username,
			},
		},
	})
}
