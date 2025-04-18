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
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/generic"
	kstorage "k8s.io/apiserver/pkg/storage"
)

var (
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
	schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}.String(): ServiceGetAttrs,
}

func GetAttrsFunc(gvr *schema.GroupVersionResource) kstorage.AttrFunc {
	if _, exist := AttrsFuncMap[gvr.String()]; exist {
		return AttrsFuncMap[gvr.String()]
	}
	return DefaultAttrsFunc
}

func ServiceGetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	service, ok := obj.(*v1.Service)
	if !ok {
		return nil, nil, fmt.Errorf("not a service")
	}
	return service.Labels, ServiceSelectableFields(service), nil
}

func ServiceSelectableFields(service *v1.Service) fields.Set {
	objectMetaFieldsSet := generic.ObjectMetaFieldsSet(&service.ObjectMeta, true)
	serviceSpecificFieldsSet := fields.Set{
		"spec.clusterIP": service.Spec.ClusterIP,
		"spec.type":      string(service.Spec.Type),
	}
	return generic.MergeFieldsSets(objectMetaFieldsSet, serviceSpecificFieldsSet)
}
