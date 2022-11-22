package poolcoordinator

import (
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	DefaultPoolScopedUserAgent = "leader-yurthub"
)
