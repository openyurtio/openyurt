/*
Copyright 2020 The OpenYurt Authors.

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

package cachemanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	hubmeta "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

var (
	ErrInMemoryCacheMiss = errors.New("in-memory cache miss")
)

// CacheManager is an adaptor to cache runtime object data into backend storage
type CacheManager interface {
	CacheResponse(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error
	QueryCache(req *http.Request) (runtime.Object, error)
	CanCacheFor(req *http.Request) bool
	DeleteKindFor(gvr schema.GroupVersionResource) error
}

type cacheManager struct {
	sync.RWMutex
	storage               StorageWrapper
	serializerManager     *serializer.SerializerManager
	restMapperManager     *hubmeta.RESTMapperManager
	cacheAgents           sets.String
	listSelectorCollector map[storage.Key]string
	sharedFactory         informers.SharedInformerFactory
	inMemoryCache         map[string]runtime.Object
}

// NewCacheManager creates a new CacheManager
func NewCacheManager(
	storagewrapper StorageWrapper,
	serializerMgr *serializer.SerializerManager,
	restMapperMgr *hubmeta.RESTMapperManager,
	sharedFactory informers.SharedInformerFactory,
) CacheManager {
	cm := &cacheManager{
		storage:               storagewrapper,
		serializerManager:     serializerMgr,
		restMapperManager:     restMapperMgr,
		cacheAgents:           sets.NewString(util.DefaultCacheAgents...),
		listSelectorCollector: make(map[storage.Key]string),
		sharedFactory:         sharedFactory,
	}

	NewCacheAgents(sharedFactory, storagewrapper)
	return cm
}

// CacheResponse cache response of request into backend storage
func (cm *cacheManager) CacheResponse(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	if isWatch(ctx) {
		return cm.saveWatchObject(ctx, info, prc, stopCh)
	}

	var buf bytes.Buffer
	n, err := buf.ReadFrom(prc)
	if err != nil {
		klog.Errorf("failed to cache response, %v", err)
		return err
	} else if n == 0 {
		err := fmt.Errorf("read 0-length data from response, %s", util.ReqInfoString(info))
		klog.Errorf("failed to cache response, %v", err)
		return err
	} else {
		klog.V(5).Infof("cache %d bytes from response for %s", n, util.ReqInfoString(info))
	}

	if isList(ctx) {
		return cm.saveListObject(ctx, info, buf.Bytes())
	}

	return cm.saveOneObject(ctx, info, buf.Bytes())
}

// QueryCache get runtime object from backend storage for request
func (cm *cacheManager) QueryCache(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || info == nil || info.Resource == "" {
		return nil, fmt.Errorf("failed to get request info")
	}
	comp, _ := util.ClientComponentFrom(ctx)
	// query in-memory cache first
	var isInMemoryCache = isInMemeoryCache(ctx)
	var isInMemoryCacheMiss bool
	if isInMemoryCache {
		if obj, err := cm.queryInMemeryCache(info); err != nil {
			if err == ErrInMemoryCacheMiss {
				isInMemoryCacheMiss = true
				klog.V(4).Infof("in-memory cache miss when handling request %s, fall back to storage query", util.ReqString(req))
			} else {
				klog.Errorf("cannot query in-memory cache for reqInfo %s, %v,", util.ReqInfoString(info), err)
			}
		} else {
			klog.V(4).Infof("in-memory cache hit when handling request %s", util.ReqString(req))
			return obj, nil
		}
	}

	// fall back to normal query
	if info.IsResourceRequest && info.Verb == "list" {
		return cm.queryListObject(req)
	} else if info.IsResourceRequest && (info.Verb == "get" || info.Verb == "patch" || info.Verb == "update") {
		key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
			Component: comp,
			Namespace: info.Namespace,
			Name:      info.Name,
			Resources: info.Resource,
		})
		if err != nil {
			return nil, err
		}

		klog.V(4).Infof("component: %s try to get key: %s", comp, key)
		obj, err := cm.storage.Get(key)
		// When yurthub restart, the data stored in in-memory cache will loss,
		// we need to rebuild the in-memory cache with backend consistent storage.
		// Note:
		// When cloud-edge network is healthy, the inMemoryCache can be updated with response from cloud side.
		// While cloud-edge network is broken, the inMemoryCache can only be full filled with data from edge cache,
		// such as local disk and pool-coordinator.
		if isInMemoryCacheMiss {
			if inMemoryCacheKey, err := inMemoryCacheKeyFunc(info); err != nil {
				klog.Errorf("cannot in-memory cache key for req %s, %v", util.ReqString(req), err)
			} else {
				cm.inMemoryCacheFor(inMemoryCacheKey, obj)
				klog.V(4).Infof("use obj from backend storage to update in-memory cache of key %s", inMemoryCacheKey)
			}
		}
		return cm.storage.Get(key)
	}

	return nil, fmt.Errorf("request(%#+v) is not supported", info)
}

func (cm *cacheManager) queryListObject(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	comp, _ := util.ClientComponentFrom(ctx)
	key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
		Component: comp,
		Namespace: info.Namespace,
		Name:      info.Name,
		Resources: info.Resource,
	})
	if err != nil {
		return nil, err
	}

	var gvk schema.GroupVersionKind
	var kind string
	// If the GVR information is not recognized, return 404 not found directly
	gvr := schema.GroupVersionResource{
		Group:    info.APIGroup,
		Version:  info.APIVersion,
		Resource: info.Resource,
	}
	if _, gvk = cm.restMapperManager.KindFor(gvr); gvk.Empty() {
		return nil, hubmeta.ErrGVRNotRecognized
	} else {
		kind = gvk.Kind
	}

	// If the GVR information is recognized, return list or empty list
	objs, err := cm.storage.List(key)
	if err != nil {
	} else if len(objs) == 0 {
		if isKubeletPodRequest(req) {
			// because at least there will be yurt-hub pod on the node.
			// if no pods in cache, maybe all of pods have been deleted by accident,
			// if empty object is returned, pods on node will be deleted by kubelet.
			// in order to prevent the influence to business, return error here so pods
			// will be kept on node.
			klog.Warningf("get 0 pods for kubelet pod request, there should be at least one, hack the response with ErrStorageNotFound error")
			return nil, storage.ErrStorageNotFound
		}
	} else {
		// If restMapper's kind and object's kind are inconsistent, use the object's kind
		objKind := objs[0].GetObjectKind().GroupVersionKind().Kind
		if kind != objKind {
			klog.Warningf("The restMapper's kind(%v) and object's kind(%v) are inconsistent ", kind, objKind)
			kind = objKind
		}
	}

	var listObj runtime.Object
	listGvk := schema.GroupVersionKind{
		Group:   info.APIGroup,
		Version: info.APIVersion,
		Kind:    kind + "List",
	}
	if scheme.Scheme.Recognizes(listGvk) {
		listObj, err = scheme.Scheme.New(listGvk)
		if err != nil {
			klog.Errorf("failed to create list object(%v), %v", listGvk, err)
			return nil, err
		}
	} else {
		listObj = new(unstructured.UnstructuredList)
		listObj.GetObjectKind().SetGroupVersionKind(listGvk)
	}

	listRv := 0
	rvStr := ""
	rvInt := 0
	accessor := meta.NewAccessor()
	for i := range objs {
		rvStr, _ = accessor.ResourceVersion(objs[i])
		rvInt, _ = strconv.Atoi(rvStr)
		if rvInt > listRv {
			listRv = rvInt
		}
	}

	if err := meta.SetList(listObj, objs); err != nil {
		klog.Errorf("failed to meta set list with %d objects, %v", len(objs), err)
		return nil, err
	}

	accessor.SetResourceVersion(listObj, strconv.Itoa(listRv))
	err = setListObjSelfLink(listObj, req)
	return listObj, err
}

func setListObjSelfLink(listObj runtime.Object, req *http.Request) error {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	clusterScoped := true
	if info.Namespace != "" {
		clusterScoped = false
	}

	prefix := "/" + path.Join(info.APIGroup, info.APIVersion)
	namer := handlers.ContextBasedNaming{
		SelfLinker:         runtime.SelfLinker(meta.NewAccessor()),
		SelfLinkPathPrefix: path.Join(prefix, info.Resource) + "/",
		SelfLinkPathSuffix: "",
		ClusterScoped:      clusterScoped,
	}

	uri, err := namer.GenerateListLink(req)
	if err != nil {
		return err
	}
	if err := namer.SetSelfLink(listObj, uri); err != nil {
		klog.Infof("Unable to set self link on object: %v", err)
	}

	return nil
}

func (cm *cacheManager) saveWatchObject(ctx context.Context, info *apirequest.RequestInfo, r io.ReadCloser, stopCh <-chan struct{}) error {
	delObjCnt := 0
	updateObjCnt := 0
	addObjCnt := 0

	comp, _ := util.ClientComponentFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	s := cm.serializerManager.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
	if s == nil {
		klog.Errorf("failed to create serializer in saveWatchObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("failed to create serializer in saveWatchObject, %s", util.ReqInfoString(info))
	}
	accessor := meta.NewAccessor()

	d, err := s.WatchDecoder(r)
	if err != nil {
		klog.Errorf("saveWatchObject ended with error, %v", err)
		return err
	}

	defer func() {
		klog.Infof("%s watch %s: %s get %d objects(add:%d/update:%d/del:%d)", comp, info.Resource, info.Path, addObjCnt+updateObjCnt+delObjCnt, addObjCnt, updateObjCnt, delObjCnt)
	}()

	for {
		watchType, obj, err := d.Decode()
		if err != nil {
			klog.V(3).Infof("%s %s watch decode ended with: %v", comp, info.Path, err)
			return err
		}

		switch watchType {
		case watch.Added, watch.Modified, watch.Deleted:
			name, err := accessor.Name(obj)
			if err != nil || name == "" {
				klog.Errorf("failed to get name of watch object, %v", err)
				continue
			}

			ns, err := accessor.Namespace(obj)
			if err != nil {
				klog.Errorf("failed to get namespace of watch object, %v", err)
				continue
			}

			key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
				Component: comp,
				Namespace: ns,
				Name:      name,
				Resources: info.Resource,
			})
			if err != nil {
				klog.Errorf("failed to get cache path, %v", err)
				continue
			}

			switch watchType {
			case watch.Added, watch.Modified:
				err = cm.saveOneObjectWithValidation(key, obj)
				if watchType == watch.Added {
					addObjCnt++
				} else {
					updateObjCnt++
				}
			case watch.Deleted:
				err = cm.storage.Delete(key)
				delObjCnt++
			default:
				// impossible go to here
			}

			if info.Resource == "pods" {
				klog.V(2).Infof("pod(%s) is %s", key, string(watchType))
			}

			if errors.Is(err, storage.ErrStorageAccessConflict) {
				klog.V(2).Infof("skip to cache watch event because key(%s) is under processing", key)
			} else if err != nil {
				klog.Errorf("failed to process watch object %s, %v", key, err)
			}
		case watch.Bookmark:
			rv, _ := accessor.ResourceVersion(obj)
			klog.Infof("get bookmark with rv %s for %s watch %s", rv, comp, info.Resource)
		case watch.Error:
			klog.Infof("unable to understand watch event %#v", obj)
		}
	}
}

func (cm *cacheManager) saveListObject(ctx context.Context, info *apirequest.RequestInfo, b []byte) error {
	comp, _ := util.ClientComponentFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	s := cm.serializerManager.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
	if s == nil {
		klog.Errorf("failed to create serializer in saveListObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("failed to create serializer in saveListObject, %s", util.ReqInfoString(info))
	}

	list, err := s.Decode(b)
	if err != nil || list == nil {
		klog.Errorf("failed to decode response %s in saveListObject, response content type: %s, requestInfo: %s, %v",
			string(b), respContentType, util.ReqInfoString(info), err)
		return err
	}

	if _, ok := list.(*metav1.Status); ok {
		klog.Infof("it's not need to cache metav1.Status")
		return nil
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		klog.Errorf("unable to understand list result %#v (%v)", list, err)
		return fmt.Errorf("unable to understand list result %#v (%w)", list, err)
	}
	klog.V(5).Infof("list items for %s is: %d", util.ReqInfoString(info), len(items))

	kind := strings.TrimSuffix(list.GetObjectKind().GroupVersionKind().Kind, "List")
	apiVersion := schema.GroupVersion{
		Group:   info.APIGroup,
		Version: info.APIVersion,
	}.String()
	accessor := meta.NewAccessor()

	// Verify if DynamicRESTMapper(which store the CRD info) needs to be updated
	if err := cm.restMapperManager.UpdateKind(schema.GroupVersionKind{Group: info.APIGroup, Version: info.APIVersion, Kind: kind}); err != nil {
		klog.Errorf("failed to update the DynamicRESTMapper %v", err)
	}

	if info.Name != "" && len(items) == 1 {
		// list with fieldSelector=metadata.name=xxx
		accessor.SetKind(items[0], kind)
		accessor.SetAPIVersion(items[0], apiVersion)
		name, _ := accessor.Name(items[0])
		ns, _ := accessor.Namespace(items[0])
		if ns == "" {
			ns = info.Namespace
		}
		key, _ := cm.storage.KeyFunc(storage.KeyBuildInfo{
			Component: comp,
			Namespace: ns,
			Name:      name,
			Resources: info.Resource,
		})
		err = cm.saveOneObjectWithValidation(key, items[0])
		if errors.Is(err, storage.ErrStorageAccessConflict) {
			klog.V(2).Infof("skip to cache list object because key(%s) is under processing", key)
			return nil
		}

		return err
	} else {
		// list all objects or with fieldselector/labelselector
		objs := make(map[storage.Key]runtime.Object)
		comp, _ := util.ClientComponentFrom(ctx)
		selector, _ := util.ListSelectorFrom(ctx)
		for i := range items {
			accessor.SetKind(items[i], kind)
			accessor.SetAPIVersion(items[i], apiVersion)
			name, _ := accessor.Name(items[i])
			ns, _ := accessor.Namespace(items[i])
			if ns == "" {
				ns = info.Namespace
			}

			key, _ := cm.storage.KeyFunc(storage.KeyBuildInfo{
				Component: comp,
				Namespace: ns,
				Name:      name,
				Resources: info.Resource,
			})
			objs[key] = items[i]
		}
		// if no objects in cloud cluster(objs is empty), it will clean the old files in the path of rootkey
		return cm.storage.ReplaceComponentList(comp, info.Resource, info.Namespace, selector, objs)
	}
}

func (cm *cacheManager) saveOneObject(ctx context.Context, info *apirequest.RequestInfo, b []byte) error {
	comp, _ := util.ClientComponentFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)

	s := cm.serializerManager.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
	if s == nil {
		klog.Errorf("failed to create serializer in saveOneObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("failed to create serializer in saveOneObject, %s", util.ReqInfoString(info))
	}

	obj, err := s.Decode(b)
	if err != nil {
		klog.Errorf("failed to decode response %s in saveOneObject(respContentType:%s): %s, %v", string(b), respContentType, util.ReqInfoString(info), err)
		return err
	} else if obj == nil {
		klog.Info("failed to decode nil object. skip cache")
		return nil
	} else if _, ok := obj.(*metav1.Status); ok {
		klog.Infof("it's not need to cache metav1.Status.")
		return nil
	}

	var name string
	accessor := meta.NewAccessor()
	if isCreate(ctx) {
		name, _ = accessor.Name(obj)
	} else {
		name = info.Name
	}

	if name == "" {
		klog.Errorf("cache object have no name, %s", info.Path)
		return nil
	}

	key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
		Component: comp,
		Namespace: info.Namespace,
		Name:      name,
		Resources: info.Resource,
	})
	if err != nil {
		klog.Errorf("failed to get cache key(%s:%s:%s:%s), %v", comp, info.Resource, info.Namespace, info.Name, err)
		return err
	}

	// Verify if DynamicRESTMapper(which store the CRD info) needs to be updated
	gvk := obj.GetObjectKind().GroupVersionKind()
	if err := cm.restMapperManager.UpdateKind(gvk); err != nil {
		klog.Errorf("failed to update the DynamicRESTMapper %v", err)
	}

	if err := cm.saveOneObjectWithValidation(key, obj); err != nil {
		if !errors.Is(err, storage.ErrStorageAccessConflict) {
			return err
		}
		klog.V(2).Infof("skip to cache object because key(%s) is under processing", key)
	}

	// update the in-memory cache with cloud response
	if !isInMemeoryCache(ctx) {
		return nil
	}
	// When reaching here, it means the obj in backend storage has been updated/created successfully,
	// so we should also update the relative obj in in-memory cache.
	if inMemoryCacheKey, err := inMemoryCacheKeyFunc(info); err != nil {
		klog.Errorf("cannot get in-memorycache key of requestInfo %s, %v", util.ReqInfoString(info), err)
		return err
	} else {
		klog.V(4).Infof("update in-memory cache for %s", inMemoryCacheKey)
		cm.inMemoryCacheFor(inMemoryCacheKey, obj)
	}
	return nil
}

func (cm *cacheManager) saveOneObjectWithValidation(key storage.Key, obj runtime.Object) error {
	accessor := meta.NewAccessor()
	if isNotAssignedPod(obj) {
		ns, _ := accessor.Namespace(obj)
		name, _ := accessor.Name(obj)
		return fmt.Errorf("pod(%s/%s) is not assigned to a node, skip cache it", ns, name)
	}

	newRv, err := accessor.ResourceVersion(obj)
	if err != nil {
		klog.Errorf("failed to get new object resource version for %s, %v", key, err)
		return err
	}

	newRvUint, _ := strconv.ParseUint(newRv, 10, 64)
	_, err = cm.storage.Update(key, obj, newRvUint)
	if err != nil {
		if err != storage.ErrStorageAccessConflict {
			return cm.storage.Create(key, obj)
		}
		return err
	}
	return nil
}

func (cm *cacheManager) inMemoryCacheFor(key string, obj runtime.Object) {
	cm.Lock()
	defer cm.Unlock()
	cm.inMemoryCache[key] = obj
}

// isNotAssignedPod check pod is assigned to node or not
// when delete pod of statefulSet, kubelet may get pod unassigned.
func isNotAssignedPod(obj runtime.Object) bool {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return false
	}

	if pod.Spec.NodeName == "" {
		return true
	}

	return false
}

func isList(ctx context.Context) bool {
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		return info.Verb == "list"
	}

	return false
}

func isWatch(ctx context.Context) bool {
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		return info.Verb == "watch"
	}

	return false
}

func isCreate(ctx context.Context) bool {
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		return info.Verb == "create"
	}

	return false
}

// CanCacheFor checks response of request can be cached or not
// the following request is not supported to cache response
// 1. component is not set
// 2. delete/deletecollection/proxy request
// 3. sub-resource request but is not status
// 4. csr resource request
func (cm *cacheManager) CanCacheFor(req *http.Request) bool {
	ctx := req.Context()

	comp, ok := util.ClientComponentFrom(ctx)
	if !ok || len(comp) == 0 {
		return false
	}

	canCache, ok := util.ReqCanCacheFrom(ctx)
	if ok && canCache {
		// request with Edge-Cache header, continue verification
	} else {
		cm.RLock()
		if !cm.cacheAgents.HasAny("*", comp) {
			cm.RUnlock()
			return false
		}
		cm.RUnlock()
	}

	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || info == nil {
		return false
	}

	if !info.IsResourceRequest {
		return false
	}

	if info.Verb == "delete" || info.Verb == "deletecollection" || info.Verb == "proxy" {
		return false
	}

	if info.Subresource != "" && info.Subresource != "status" {
		return false
	}

	return true
}

// DeleteKindFor is used to delete the invalid Kind(which is not registered in the cloud)
func (cm *cacheManager) DeleteKindFor(gvr schema.GroupVersionResource) error {
	return cm.restMapperManager.DeleteKindFor(gvr)
}

func (cm *cacheManager) queryInMemeryCache(reqInfo *apirequest.RequestInfo) (runtime.Object, error) {
	key, err := inMemoryCacheKeyFunc(reqInfo)
	if err != nil {
		return nil, err
	}

	cm.RLock()
	defer cm.RUnlock()
	obj, ok := cm.inMemoryCache[key]
	if !ok {
		return nil, ErrInMemoryCacheMiss
	}

	return obj, nil
}

func isKubeletPodRequest(req *http.Request) bool {
	ctx := req.Context()
	comp, ok := util.ClientComponentFrom(ctx)
	if !ok || comp != "kubelet" {
		return false
	}

	if reqInfo, ok := apirequest.RequestInfoFrom(ctx); ok {
		return reqInfo.Resource == "pods"
	}

	return false
}

// isInMemmoryCache verify if the response of the request should be cached in-memory.
// In order to accelerate kubelet get node and lease object, we cache them
func isInMemeoryCache(reqCtx context.Context) bool {
	var comp, resource string
	var reqInfo *apirequest.RequestInfo
	var ok bool
	if comp, ok = util.ClientComponentFrom(reqCtx); !ok {
		return false
	}
	if reqInfo, ok = apirequest.RequestInfoFrom(reqCtx); !ok {
		return false
	}

	resource = reqInfo.Resource
	if comp == "kubelet" && (resource == "nodes" || resource == "leases") {
		return true
	}
	return false
}

func inMemoryCacheKeyFunc(reqInfo *apirequest.RequestInfo) (string, error) {
	res, ns, name := reqInfo.Resource, reqInfo.Namespace, reqInfo.Name
	if res == "" {
		return "", fmt.Errorf("resource should not be empty")
	}
	if name == "" {
		// currently only signal resource can be cached in memory
		return "", fmt.Errorf("name cannot be empty")
	}

	key := filepath.Join(res, ns, name)
	return key, nil
}
