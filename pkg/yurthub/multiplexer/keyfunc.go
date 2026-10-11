/*
Copyright 2025 The OpenYurt Authors.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

// resourcePrefix is the key prefix all objects of a resource live under in the
// multiplexer cache. Since k8s v1.36 the cacher requires a non-empty prefix and
// rejects keys that don't start with it, so every key below is built on top of it.
func resourcePrefix(gvr *schema.GroupVersionResource) string {
	if len(gvr.Group) == 0 {
		return "/" + gvr.Resource
	}
	return "/" + gvr.Group + "/" + gvr.Resource
}

func newKeyFunc(prefix string) func(obj runtime.Object) (string, error) {
	return func(obj runtime.Object) (string, error) {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return "", err
		}

		name := accessor.GetName()
		if len(name) == 0 {
			return "", apierrors.NewBadRequest("Name parameter required.")
		}

		ns := accessor.GetNamespace()
		if len(ns) == 0 {
			return prefix + "/" + name, nil
		}
		return prefix + "/" + ns + "/" + name, nil
	}
}

func newResourceKeyRootFunc(prefix string) func(ctx context.Context) string {
	return func(ctx context.Context) string {
		ns, ok := genericapirequest.NamespaceFrom(ctx)
		if ok {
			return prefix + "/" + ns
		}

		return prefix
	}
}

func newResourceKeyFunc(prefix string) func(ctx context.Context, name string) (string, error) {
	return func(ctx context.Context, name string) (string, error) {
		ns, ok := genericapirequest.NamespaceFrom(ctx)
		if ok {
			return prefix + "/" + ns + "/" + name, nil
		}

		return prefix + "/" + name, nil
	}
}
