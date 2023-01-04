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
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

var (
	PoolScopedResources = map[schema.GroupVersionResource]struct{}{
		{Group: "", Version: "v1", Resource: "endpoints"}:                      {},
		{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}: {},
	}

	UploadResourcesKeyBuildInfo = map[storage.KeyBuildInfo]struct{}{
		{Component: "kubelet", Resources: "pods", Group: "", Version: "v1"}:  {},
		{Component: "kubelet", Resources: "nodes", Group: "", Version: "v1"}: {},
	}
)

const (
	DefaultPoolScopedUserAgent           = "leader-yurthub"
	PoolCoordinatorClientSecretName      = "pool-coordinator-yurthub-certs"
	PoolCoordinatorClientSecretNamespace = "kube-system"
)
