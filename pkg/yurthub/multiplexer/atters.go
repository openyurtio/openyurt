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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/pkg/registry/core/service"
)

var (
	KeyFunc = func(obj runtime.Object) (string, error) {
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

	DefaultAttrsFunc = func(obj runtime.Object) (labels.Set, fields.Set, error) {
		metadata, err := meta.Accessor(obj)
		if err != nil {
			return nil, nil, err
		}

		var fieldSet fields.Set
		if len(metadata.GetNamespace()) > 0 {
			fieldSet = fields.Set{
				"metadata.name":      metadata.GetName(),
				"metadata.namespace": metadata.GetNamespace(),
			}
		} else {
			fieldSet = fields.Set{
				"metadata.name": metadata.GetName(),
			}
		}

		return labels.Set(metadata.GetLabels()), fieldSet, nil
	}
)

var AttrsFuncMap = map[string]kstorage.AttrFunc{
	schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}.String(): service.GetAttrs,
}

func GetAttrsFunc(gvr *schema.GroupVersionResource) kstorage.AttrFunc {
	if _, exist := AttrsFuncMap[gvr.String()]; exist {
		return AttrsFuncMap[gvr.String()]
	}
	return DefaultAttrsFunc
}
