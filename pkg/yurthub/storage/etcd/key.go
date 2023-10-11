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

package etcd

import (
	"errors"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/validation/path"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
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
	comp string
	path string
	gvr  schema.GroupVersionResource
}

func (k storageKey) Key() string {
	return k.path
}

func (k storageKey) component() string {
	return k.comp
}

func (s *etcdStorage) KeyFunc(info storage.KeyBuildInfo) (storage.Key, error) {
	if info.Component == "" {
		return nil, storage.ErrEmptyComponent
	}
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

	gvr := schema.GroupVersionResource{Group: info.Group, Version: info.Version, Resource: info.Resources}
	if isSchema := hubmeta.IsSchemeResource(gvr); !isSchema {
		group := info.Group
		path = filepath.Join(s.prefix, group, resource, info.Namespace, info.Name)
	}

	return storageKey{
		comp: info.Component,
		path: path,
		gvr: schema.GroupVersionResource{
			Group:    info.Group,
			Version:  info.Version,
			Resource: info.Resources,
		},
	}, nil
}
