/*
Copyright 2024 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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
	"net/http"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	kstorage "k8s.io/apiserver/pkg/storage"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func (sp *shareProxy) shareList(w http.ResponseWriter, r *http.Request, gvr *schema.GroupVersionResource) {
	scope, err := sp.getReqScope(gvr)
	if err != nil {
		util.Err(errors.Wrapf(err, "failed to get request scope"), w, r)
		return
	}

	listOpts, err := sp.decodeListOptions(r, scope)
	if err != nil {
		util.Err(errors.Wrapf(err, "failed to decode list options, url: %v", r.URL), w, r)
		return
	}

	storageOpts, err := sp.storageOpts(listOpts, gvr)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	obj, err := sp.listObject(r, gvr, storageOpts)
	if err != nil {
		util.Err(err, w, r)
		return
	}

	util.WriteObject(http.StatusOK, obj, w, r)
}

func (sp *shareProxy) listObject(r *http.Request, gvr *schema.GroupVersionResource, storageOpts *kstorage.ListOptions) (runtime.Object, error) {
	rc, _, err := sp.cacheManager.ResourceCache(gvr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get resource cache")
	}

	obj, err := sp.newListObject(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to new list object")
	}

	key, err := sp.getCacheKey(r, storageOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get cache key")
	}

	if err := rc.GetList(r.Context(), key, *storageOpts, obj); err != nil {
		return nil, errors.Wrapf(err, "failed to get list from cache")
	}

	if obj, err = sp.filterListObject(obj, sp.filterMgr.FindObjectFilters(r)); err != nil {
		return nil, errors.Wrapf(err, "failed to filter list object")
	}

	return obj, nil
}

func (sp *shareProxy) newListObject(gvr *schema.GroupVersionResource) (runtime.Object, error) {
	rcc, err := sp.cacheManager.ResourceCacheConfig(gvr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get resource cache config")
	}

	return rcc.NewListFunc(), nil
}

func (sp *shareProxy) getCacheKey(r *http.Request, storageOpts *kstorage.ListOptions) (string, error) {
	if ns := sp.getNamespace(r); len(ns) > 0 {
		return sp.getNamespaceScopedCacheKey(r, storageOpts)
	}

	return sp.getClusterScopedCacheKey(r, storageOpts)
}

func (sp *shareProxy) getNamespaceScopedCacheKey(r *http.Request, storageOpts *kstorage.ListOptions) (string, error) {
	ctx := request.WithNamespace(r.Context(), sp.getNamespace(r))

	if name, ok := storageOpts.Predicate.MatchesSingle(); ok {
		return registry.NamespaceKeyFunc(ctx, "", name)
	}

	return registry.NamespaceKeyRootFunc(ctx, ""), nil
}

func (sp *shareProxy) getNamespace(r *http.Request) string {
	requestInfo, ok := request.RequestInfoFrom(r.Context())
	if !ok {
		return ""
	}
	return requestInfo.Namespace
}

func (sp *shareProxy) getClusterScopedCacheKey(r *http.Request, storageOpts *kstorage.ListOptions) (string, error) {
	if name, ok := storageOpts.Predicate.MatchesSingle(); ok {
		return registry.NoNamespaceKeyFunc(r.Context(), "", name)
	}

	return "", nil
}

func (sp *shareProxy) filterListObject(obj runtime.Object, filter filter.ObjectFilter) (runtime.Object, error) {
	if filter == nil {
		return obj, nil
	}

	items, err := meta.ExtractList(obj)

	if err != nil || len(items) == 0 {
		return filter.Filter(obj, sp.stop), nil
	}

	list := make([]runtime.Object, 0)
	for _, item := range items {
		newObj := filter.Filter(item, sp.stop)
		if !yurtutil.IsNil(newObj) {
			list = append(list, newObj)
		}
	}

	if err = meta.SetList(obj, list); err != nil {
		klog.Warningf("filter %s doesn't work correctly, couldn't set list, %v.", filter.Name(), err)
	}

	return obj, nil
}
