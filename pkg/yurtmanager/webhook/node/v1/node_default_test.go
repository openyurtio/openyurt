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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func TestDefault(t *testing.T) {
	testcases := map[string]struct {
		node    runtime.Object
		pool    *appsv1beta1.NodePool
		errCode int
		errMsg  string
	}{
		"it is not a node": {
			node:    &corev1.Pod{},
			errCode: http.StatusBadRequest,
		},
		"it is a orphan node": {
			node:    &corev1.Node{},
			errCode: 0,
		},
		"nodepool doesn't exist": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "hangzhou",
					},
				},
			},
			errCode: 0,
			errMsg:  "not found",
		},
		"add labels for node": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						projectinfo.GetNodePoolLabel(): "shanghai",
					},
				},
			},
			pool: &appsv1beta1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shanghai",
				},
				Spec: appsv1beta1.NodePoolSpec{
					Type:        appsv1beta1.Edge,
					HostNetwork: true,
				},
			},
			errCode: 0,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := clientgoscheme.AddToScheme(scheme); err != nil {
				t.Fatal("Fail to add kubernetes clint-go custom resource")
			}
			apis.AddToScheme(scheme)

			var c client.Client
			if tc.pool != nil {
				c = fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(tc.pool).Build()
			} else {
				c = fakeclient.NewClientBuilder().WithScheme(scheme).Build()
			}

			w := &NodeHandler{Client: c}
			err := w.Default(context.TODO(), tc.node, admission.Request{})
			if err != nil {
				if tc.errCode != 0 {
					statusErr := err.(*errors.StatusError)
					if tc.errCode != int(statusErr.Status().Code) {
						t.Errorf("Expected error code %d, got %v", tc.errCode, err)
						return
					}
				}

				if len(tc.errMsg) != 0 {
					if !strings.Contains(err.Error(), tc.errMsg) {
						t.Errorf("Expected error msg %s, got %s", tc.errMsg, err.Error())
						return
					}
				}
			} else if tc.errCode != 0 && len(tc.errMsg) != 0 {
				t.Errorf("Expected error code %d, errmsg %s, got %v", tc.errCode, tc.errMsg, err)
			}
		})
	}
}
