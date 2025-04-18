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
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

var keyFunc = func(obj runtime.Object) (string, error) {
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
		return "/" + name, nil
	}
	return "/" + ns + "/" + name, nil
}

func resourceKeyRootFunc(ctx context.Context) string {
	ns, ok := genericapirequest.NamespaceFrom(ctx)
	if ok {
		return "/" + ns
	}

	return "/"
}

func resourceKeyFunc(ctx context.Context, name string) (string, error) {
	ns, ok := genericapirequest.NamespaceFrom(ctx)
	if ok {
		return "/" + ns + "/" + name, nil
	}

	return "/" + name, nil
}
