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
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

var s = etcdStorage{
	prefix: "/registry",
}

var keyFunc = s.KeyFunc

func TestKeyFunc(t *testing.T) {
	cases := map[string]struct {
		info storage.KeyBuildInfo
		key  string
		err  error
	}{
		"core group normal case": {
			info: storage.KeyBuildInfo{
				Component: "kubelet",
				Group:     "",
				Resources: "pods",
				Version:   "v1",
				Namespace: "test",
				Name:      "test-pod",
			},
			key: "/registry/pods/test/test-pod",
		},

		"special prefix for node resource": {
			info: storage.KeyBuildInfo{
				Component: "kubelet",
				Group:     "",
				Resources: "nodes",
				Version:   "v1",
				Namespace: "",
				Name:      "test-node",
			},
			key: "/registry/minions/test-node",
		},
		"not core group": {
			info: storage.KeyBuildInfo{
				Component: "kubelet",
				Group:     "apps",
				Resources: "deployments",
				Version:   "v1",
				Namespace: "test",
				Name:      "test-deploy",
			},
			key: "/registry/deployments/test/test-deploy",
		},
		"special prefix for service resource": {
			info: storage.KeyBuildInfo{
				Component: "kube-proxy",
				Group:     "networking.k8s.io",
				Resources: "ingresses",
				Version:   "v1",
				Namespace: "test",
				Name:      "test-ingress",
			},
			key: "/registry/ingress/test/test-ingress",
		},
		"empty resources": {
			info: storage.KeyBuildInfo{
				Component: "yurthub",
				Group:     "",
				Resources: "",
				Version:   "v1",
				Namespace: "",
				Name:      "",
			},
			err: storage.ErrEmptyResource,
		},
		"empty component": {
			info: storage.KeyBuildInfo{
				Group:     "",
				Resources: "nodes",
				Version:   "v1",
				Namespace: "",
				Name:      "test-node",
			},
			err: storage.ErrEmptyComponent,
		},
	}

	for n, c := range cases {
		key, err := keyFunc(c.info)
		if err != c.err {
			t.Errorf("unexpected error in case %s, want: %v, got: %v", n, c.err, err)
			continue
		}
		if err != nil {
			continue
		}
		if key.Key() != c.key {
			t.Errorf("unexpected key in case %s, want: %s, got: %s", n, c.key, key.Key())
		}
	}
}
