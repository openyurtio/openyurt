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

package constants

import (
	v1rbac "k8s.io/api/rbac/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

var (
	UploadResourcesKeyBuildInfo = map[storage.KeyBuildInfo]struct{}{
		{Component: "kubelet", Resources: "pods", Group: "", Version: "v1"}:  {},
		{Component: "kubelet", Resources: "nodes", Group: "", Version: "v1"}: {},
	}
	CoordinatorAPIServerClusterRoleBinding = v1rbac.ClusterRoleBinding{
		TypeMeta: v1meta.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: v1meta.ObjectMeta{
			Name: "openyurt:yurt-coordinator:apiserver",
		},
		Subjects: []v1rbac.Subject{
			{
				Kind:     "User",
				APIGroup: "rbac.authorization.k8s.io",
				Name:     "openyurt:yurt-coordinator:apiserver",
			},
		},
		RoleRef: v1rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:kubelet-api-admin",
		},
	}
)

const (
	DefaultPoolScopedUserAgent      = "leader-yurthub"
	YurtCoordinatorClientSecretName = "yurt-coordinator-yurthub-certs"
)
