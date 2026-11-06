/*
Copyright 2026 The OpenYurt Authors.

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

package multiplexer

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestResourcePrefix(t *testing.T) {
	cases := map[string]struct {
		gvr    *schema.GroupVersionResource
		expect string
	}{
		"core group":  {gvr: &schema.GroupVersionResource{Version: "v1", Resource: "services"}, expect: "/services"},
		"named group": {gvr: &schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}, expect: "/discovery.k8s.io/endpointslices"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := resourcePrefix(tc.gvr); got != tc.expect {
				t.Errorf("expected %q, got %q", tc.expect, got)
			}
		})
	}
}

func TestKeyFuncsUsePrefix(t *testing.T) {
	prefix := resourcePrefix(serviceGVR)
	nsCtx := genericapirequest.WithNamespace(context.Background(), "kube-system")

	key, err := newKeyFunc(prefix)(&v1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns"}})
	if err != nil || key != "/services/kube-system/coredns" {
		t.Errorf("object key: got %q, %v", key, err)
	}
	if _, err := newKeyFunc(prefix)(&v1.Service{}); err == nil {
		t.Errorf("expected error for object without name")
	}

	if got := newResourceKeyRootFunc(prefix)(context.Background()); got != "/services" {
		t.Errorf("root key without namespace: got %q", got)
	}
	if got := newResourceKeyRootFunc(prefix)(nsCtx); got != "/services/kube-system" {
		t.Errorf("root key with namespace: got %q", got)
	}

	key, err = newResourceKeyFunc(prefix)(nsCtx, "coredns")
	if err != nil || key != "/services/kube-system/coredns" {
		t.Errorf("resource key: got %q, %v", key, err)
	}
}
