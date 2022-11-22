package etcd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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
	isRootKey bool
	path      string
}

func (k *storageKey) Key() string {
	return k.path
}

func (k *storageKey) IsRootKey() bool {
	return k.isRootKey
}

func (k *storageKey) String() string {
	return fmt.Sprintf("%s:%v", k.path, k.isRootKey)
}

func (k *storageKey) From(s string) error {
	elems := strings.Split(s, ":")
	if len(elems) != 2 {
		return fmt.Errorf("failed to parse key %s, invalid format", s)
	}
	k.path = elems[0]
	if elems[1] == "true" {
		k.isRootKey = true
	} else {
		k.isRootKey = false
	}
	return nil
}

func (s *etcdStorage) KeyFunc(info storage.KeyBuildInfo) (storage.Key, error) {
	isRoot := false
	if info.Resources == "" {
		return nil, storage.ErrEmptyResource
	}
	if errStrs := path.IsValidPathSegmentName(info.Name); len(errStrs) != 0 {
		return nil, errors.New(errStrs[0])
	}
	if info.Name == "" {
		isRoot = true
	}

	resource := info.Resources
	gr := schema.GroupResource{Group: info.Group, Resource: info.Resources}
	if override, ok := SpecialDefaultResourcePrefixes[gr]; ok {
		resource = override
	}

	path := filepath.Join(s.prefix, resource, info.Namespace, info.Name)

	return &storageKey{
		path:      path,
		isRootKey: isRoot,
	}, nil
}
