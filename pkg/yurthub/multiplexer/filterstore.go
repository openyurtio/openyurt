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

	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	storeerr "k8s.io/apiserver/pkg/storage/errors"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type filterStore struct {
	store *registry.Store
	gvr   *schema.GroupVersionResource
}

func (fs *filterStore) New() runtime.Object {
	return fs.store.New()
}

func (fs *filterStore) NewList() runtime.Object {
	return fs.store.NewList()
}

func (fs *filterStore) Destroy() {
	fs.store.Destroy()
}

func (fs *filterStore) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	if options.ResourceVersion == "" {
		options.ResourceVersion = "0"
	}

	result, err := fs.store.List(ctx, options)
	if err != nil {
		return result, err
	}

	filters, ok := util.ObjectFilterFrom(ctx)
	if !ok {
		return result, nil
	}

	if result, err = fs.filterListObject(ctx, result, filters); err != nil {
		return nil, storeerr.InterpretListError(err, fs.qualifiedResourceFromContext(ctx))
	}

	return result, nil
}

func (fs *filterStore) filterListObject(ctx context.Context, obj runtime.Object, filter filter.ObjectFilter) (runtime.Object, error) {
	if yurtutil.IsNil(filter) {
		return obj, nil
	}

	items, err := meta.ExtractList(obj)

	if err != nil || len(items) == 0 {
		return filter.Filter(obj, ctx.Done()), nil
	}

	list := make([]runtime.Object, 0)
	for _, item := range items {
		newObj := filter.Filter(item, ctx.Done())
		if !yurtutil.IsNil(newObj) {
			list = append(list, newObj)
		}
	}

	if err = meta.SetList(obj, list); err != nil {
		klog.Warningf("filter %s doesn't work correctly, couldn't set list, %v.", filter.Name(), err)
	}

	return obj, nil
}

func (fs *filterStore) qualifiedResourceFromContext(ctx context.Context) schema.GroupResource {
	if info, ok := request.RequestInfoFrom(ctx); ok {
		return schema.GroupResource{Group: info.APIGroup, Resource: info.Resource}
	}
	// some implementations access storage directly and thus the context has no RequestInfo
	return fs.gvr.GroupResource()
}

func (fs *filterStore) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	result, err := fs.store.Watch(ctx, options)
	if err != nil {
		return result, err
	}

	filters, ok := util.ObjectFilterFrom(ctx)
	if !ok {
		return result, nil
	}
	return newFilterWatch(result, filters), nil
}

func (fs *filterStore) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return rest.NewDefaultTableConvertor(fs.gvr.GroupResource()).ConvertToTable(ctx, object, tableOptions)
}

func (fs *filterStore) ReadinessCheck() error {
	return fs.store.ReadinessCheck()
}

func getMatchFunc(gvr *schema.GroupVersionResource) func(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return func(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
		return storage.SelectionPredicate{
			Label:    label,
			Field:    field,
			GetAttrs: GetAttrsFunc(gvr),
		}
	}
}
