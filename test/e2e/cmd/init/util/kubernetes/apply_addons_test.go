/*
Copyright 2022 The OpenYurt Authors.

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

package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

func TestDeployYurthubSetting(t *testing.T) {
	fakeKubeClient := clientsetfake.NewSimpleClientset()
	err := DeployYurthubSetting(fakeKubeClient)
	if err != nil {
		t.Logf("falied deploy yurt controller manager")
	}
}

func TestDeleteYurthubSetting(t *testing.T) {
	cases := []struct {
		clusterRoleObj        *rbacv1.ClusterRole
		clusterRoleBindingObj *rbacv1.ClusterRoleBinding
		configObj             *corev1.ConfigMap
		want                  error
	}{
		{
			clusterRoleObj: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yurt-hub",
				},
			},
			clusterRoleBindingObj: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yurt-hub",
				},
			},

			configObj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Name:      "yurt-hub-cfg",
				},
			},
			want: nil,
		},
	}
	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset(v.configObj)
		err := DeleteYurthubSetting(fakeKubeClient)
		if err != v.want {
			t.Errorf("failed to delete yurthub setting")
		}
	}
}
