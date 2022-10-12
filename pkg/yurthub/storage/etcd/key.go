package etcd

import (
	"errors"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/validation/path"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

// SpecialDefaultResourcePrefixes are prefixes compiled into Kubernetes.
// refer to SpecialDefaultResourcePrefixes in k8s.io/pkg/kubeapiserver/default_storage_factory_builder.go
var SpecialDefaultResourcePrefixes = map[schema.GroupResource]string{
	{Group: "", Resource: "replicationcontrollers"}:        "controllers",
	{Group: "", Resource: "endpoints"}:                     "services/endpoints",
	{Group: "", Resource: "nodes"}:                         "minions",
	{Group: "", Resource: "services"}:                      "services/specs",
	{Group: "extensions", Resource: "ingresses"}:           "ingress",
	{Group: "networking.k8s.io", Resource: "ingresses"}:    "ingress",
	{Group: "extensions", Resource: "podsecuritypolicies"}: "podsecuritypolicy",
	{Group: "policy", Resource: "podsecuritypolicies"}:     "podsecuritypolicy",
}

type storageKey struct {
	path string
}

func (k storageKey) Key() string {
	return k.path
}

func (s *etcdStorage) KeyFunc(info storage.KeyBuildInfo) (storage.Key, error) {
	if info.Resources == "" {
		return nil, storage.ErrEmptyResource
	}
	if errStrs := path.IsValidPathSegmentName(info.Name); len(errStrs) != 0 {
		return nil, errors.New(errStrs[0])
	}

	resource := info.Resources
	gr := schema.GroupResource{Group: info.Group, Resource: info.Resources}
	if override, ok := SpecialDefaultResourcePrefixes[gr]; ok {
		resource = override
	}

	path := filepath.Join(s.prefix, resource, info.Namespace, info.Name)

	return storageKey{
		path: path,
	}, nil
}
