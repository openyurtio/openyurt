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
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversionscheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
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
	ErrNotNodeOrLease    = errors.New("resource is not node or lease")

	nonCacheableResources = map[string]struct{}{
		"certificatesigningrequests": {},
		"subjectaccessreviews":       {},
	}
)

// CacheManager is an adaptor to cache runtime object data into backend storage
type CacheManager interface {
	CacheResponse(req *http.Request, prc io.ReadCloser, stopCh <-chan struct{}) error
	QueryCache(req *http.Request) (runtime.Object, error)
	CanCacheFor(req *http.Request) bool
	DeleteKindFor(gvr schema.GroupVersionResource) error
	QueryCacheResult() CacheResult
}

type CacheResult struct {
	Length int
	Msg    string
}

type cacheManager struct {
	sync.RWMutex
	storage               StorageWrapper
	serializerManager     *serializer.SerializerManager
	restMapperManager     *hubmeta.RESTMapperManager
	cacheAgents           *CacheAgent
	listSelectorCollector map[storage.Key]string
	inMemoryCache         map[string]runtime.Object
}

// NewCacheManager creates a new CacheManager
func NewCacheManager(
	storagewrapper StorageWrapper,
	serializerMgr *serializer.SerializerManager,
	restMapperMgr *hubmeta.RESTMapperManager,
	sharedFactory informers.SharedInformerFactory,
) CacheManager {
	cacheAgents := NewCacheAgents(sharedFactory, storagewrapper)
	cm := &cacheManager{
		storage:               storagewrapper,
		serializerManager:     serializerMgr,
		cacheAgents:           cacheAgents,
		restMapperManager:     restMapperMgr,
		listSelectorCollector: make(map[storage.Key]string),
		inMemoryCache:         make(map[string]runtime.Object),
	}
	return cm
}

func (cm *cacheManager) QueryCacheResult() CacheResult {
	length, msg := cm.storage.GetCacheResult()
	return CacheResult{
		Length: length,
		Msg:    msg,
	}
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
		klog.Errorf("could not cache response, %v", err)
		return err
	} else if n == 0 {
		err := fmt.Errorf("read 0-length data from response, %s", util.ReqInfoString(info))
		klog.Errorf("could not cache response, %v", err)
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
		return nil, fmt.Errorf("could not get request info for request %s", util.ReqString(req))
	}
	if !info.IsResourceRequest {
		return nil, fmt.Errorf("could not QueryCache for getting non-resource request %s", util.ReqString(req))
	}

	switch info.Verb {
	case "list":
		return cm.queryListObject(req)
	case "get", "patch", "update":
		return cm.queryOneObject(req)
	default:
		return nil, fmt.Errorf("could not QueryCache, unsupported verb %s of request %s", info.Verb, util.ReqString(req))
	}
}

// TODO: Consider if we need accelerate the list query with in-memory cache. Currently, we only
// use in-memory cache in queryOneObject.
func (cm *cacheManager) queryListObject(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	comp, _ := util.ClientComponentFrom(ctx)
	key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
		Component: comp,
		Namespace: info.Namespace,
		Name:      info.Name,
		Resources: info.Resource,
		Group:     info.APIGroup,
		Version:   info.APIVersion,
	})
	if err != nil {
		return nil, err
	}

	listGvk, err := cm.prepareGvkForListObj(schema.GroupVersionResource{
		Group:    info.APIGroup,
		Version:  info.APIVersion,
		Resource: info.Resource,
	})
	if err != nil {
		klog.Errorf("could not get gvk for ListObject for req: %s, %v", util.ReqString(req), err)
		// If err is hubmeta.ErrGVRNotRecognized, the reverse proxy will set the HTTP Status Code as 404.
		return nil, err
	}
	listObj, err := generateEmptyListObjOfGVK(listGvk)
	if err != nil {
		klog.Errorf("could not create ListObj for gvk %s for req: %s, %v", listGvk.String(), util.ReqString(req), err)
		return nil, err
	}

	objs, err := cm.storage.List(key)
	if err == storage.ErrStorageNotFound && isListRequestWithNameFieldSelector(req) {
		// When the request is a list request with FieldSelector "metadata.name", we should not return error
		// when the specified resource is not found return an empty list object, to keep same as APIServer.
		return listObj, nil
	} else if err != nil {
		klog.Errorf("could not list key %s for request %s, %v", key.Key(), util.ReqString(req), err)
		return nil, err
	} else if len(objs) == 0 {
		if isKubeletPodRequest(req) {
			// because at least there will be yurt-hub pod on the node.
			// if no pods in cache, maybe all pods have been deleted by accident,
			// if empty object is returned, pods on node will be deleted by kubelet.
			// in order to prevent the influence to business, return error here so pods
			// will be kept on node.
			klog.Warningf("get 0 pods for kubelet pod request, there should be at least one, hack the response with ErrStorageNotFound error")
			return nil, storage.ErrStorageNotFound
		}
		// There's no obj to fill in the list, so just return.
		return listObj, nil
	}

	// When reach here, we assume that we've successfully get objs from storage and the elements num is not 0.
	// If restMapper's kind and object's kind are inconsistent, use the object's kind
	// TODO: Could this inconsistency happen? When and Why?
	if gotObjListKind := objs[0].GetObjectKind().GroupVersionKind().Kind + "List"; listGvk.Kind != gotObjListKind {
		klog.Warningf("The restMapper's kind(%v) and object's kind(%v) are inconsistent ", listGvk.Kind, gotObjListKind)
		listGvk.Kind = gotObjListKind
		if listObj, err = generateEmptyListObjOfGVK(listGvk); err != nil {
			klog.Errorf("could not create list obj for req: %s, whose gvk is %s, %v", util.ReqString(req), listGvk.String(), err)
			return nil, err
		}
	}
	if err := completeListObjWithObjs(listObj, objs); err != nil {
		klog.Errorf("could not complete the list obj %s for req %s, %v", listGvk, util.ReqString(req), err)
		return nil, err
	}
	return listObj, nil
}

func (cm *cacheManager) queryOneObject(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || info == nil || info.Resource == "" {
		return nil, fmt.Errorf("could not get request info for request %s", util.ReqString(req))
	}

	comp, _ := util.ClientComponentFrom(ctx)
	// query in-memory cache first
	var isInMemoryCacheMiss bool
	if obj, err := cm.queryInMemeryCache(ctx, info); err != nil {
		if err == ErrInMemoryCacheMiss {
			isInMemoryCacheMiss = true
			klog.V(4).Infof("in-memory cache miss when handling request %s, fall back to storage query", util.ReqString(req))
		} else if err == ErrNotNodeOrLease {
			klog.V(4).Infof("resource(%s) is not node or lease, it will be found in the disk not cache", info.Resource)
		} else {
			klog.Errorf("cannot query in-memory cache for reqInfo %s, %v,", util.ReqInfoString(info), err)
		}
	} else {
		klog.V(4).Infof("in-memory cache hit when handling request %s", util.ReqString(req))
		return obj, nil
	}

	// fall back to normal query
	key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
		Component: comp,
		Namespace: info.Namespace,
		Name:      info.Name,
		Resources: info.Resource,
		Group:     info.APIGroup,
		Version:   info.APIVersion,
	})
	if err != nil {
		return nil, err
	}

	klog.V(4).Infof("component: %s try to get key: %s", comp, key.Key())
	obj, err := cm.storage.Get(key)
	if err != nil {
		klog.Errorf("could not get obj %s from storage, %v", key.Key(), err)
		return nil, err
	}
	// When yurthub restart, the data stored in in-memory cache will lose,
	// we need to rebuild the in-memory cache with backend consistent storage.
	// Note:
	// When cloud-edge network is healthy, the inMemoryCache can be updated with response from cloud side.
	// While cloud-edge network is broken, the inMemoryCache can only be fulfilled with data from edge cache,
	// such as local disk and yurt-coordinator.
	if isInMemoryCacheMiss {
		return obj, cm.updateInMemoryCache(ctx, info, obj)
	}
	return obj, nil
}

// prepareGvkForListObj will use the RESTMapper to construct the gvk of relative ListObject according to the gvr.
// If restMapperManager cannot recognize the gvr, return hubmeta.ErrGVRNotRecognized.
func (cm *cacheManager) prepareGvkForListObj(gvr schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	var kind string
	var listGvk schema.GroupVersionKind
	// If the GVR information is not recognized, return 404 not found directly
	if _, gvk := cm.restMapperManager.KindFor(gvr); gvk.Empty() {
		return listGvk, hubmeta.ErrGVRNotRecognized
	} else {
		kind = gvk.Kind
	}
	listGvk = schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    kind + "List",
	}
	return listGvk, nil
}

func completeListObjWithObjs(listObj runtime.Object, objs []runtime.Object) error {
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
		return fmt.Errorf("could not meta set list with %d objects, %v", len(objs), err)
	}

	return accessor.SetResourceVersion(listObj, strconv.Itoa(listRv))
}

func generateEmptyListObjOfGVK(listGvk schema.GroupVersionKind) (runtime.Object, error) {
	var listObj runtime.Object
	var err error
	if scheme.Scheme.Recognizes(listGvk) {
		listObj, err = scheme.Scheme.New(listGvk)
		if err != nil {
			return nil, fmt.Errorf("could not create list object(%v), %v", listGvk, err)
		}
	} else {
		listObj = new(unstructured.UnstructuredList)
		listObj.GetObjectKind().SetGroupVersionKind(listGvk)
	}

	return listObj, nil
}

func (cm *cacheManager) saveWatchObject(ctx context.Context, info *apirequest.RequestInfo, r io.ReadCloser, stopCh <-chan struct{}) error {
	delObjCnt := 0
	updateObjCnt := 0
	addObjCnt := 0

	comp, _ := util.ClientComponentFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	s := cm.serializerManager.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
	if s == nil {
		klog.Errorf("could not create serializer in saveWatchObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("could not create serializer in saveWatchObject, %s", util.ReqInfoString(info))
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
			klog.Errorf("%s %s watch decode ended with: %v", comp, info.Path, err)
			return err
		}

		switch watchType {
		case watch.Added, watch.Modified, watch.Deleted:
			name, err := accessor.Name(obj)
			if err != nil || name == "" {
				klog.Errorf("could not get name of watch object, %v", err)
				continue
			}

			ns, err := accessor.Namespace(obj)
			if err != nil {
				klog.Errorf("could not get namespace of watch object, %v", err)
				continue
			}
			key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
				Component: comp,
				Namespace: ns,
				Name:      name,
				Resources: info.Resource,
				Group:     info.APIGroup,
				Version:   info.APIVersion,
			})
			if err != nil {
				klog.Errorf("could not get cache path, %v", err)
				continue
			}

			switch watchType {
			case watch.Added, watch.Modified:
				err = cm.storeObjectWithKey(key, obj)
				if watchType == watch.Added {
					addObjCnt++
				} else {
					updateObjCnt++
				}
				errMsg := cm.updateInMemoryCache(ctx, info, obj)
				if errMsg != nil {
					klog.Errorf("failed to update cache, %v", errMsg)
				}
			case watch.Deleted:
				err = cm.storage.Delete(key)
				delObjCnt++
				// for now, If it's a delete request, no need to modify the inmemory cache,
				// because currently, there shouldn't be any delete requests for nodes or leases.
			default:
				// impossible go here
			}

			if info.Resource == "pods" {
				klog.V(2).Infof("pod(%s) is %s", key.Key(), string(watchType))
			}

			if err != nil {
				klog.Errorf("could not process watch object %s, %v", key.Key(), err)
			}
		case watch.Bookmark:
			rv, _ := accessor.ResourceVersion(obj)
			klog.V(4).Infof("get bookmark with rv %s for %s watch %s", rv, comp, info.Resource)
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
		klog.Errorf("could not create serializer in saveListObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("could not create serializer in saveListObject, %s", util.ReqInfoString(info))
	}

	list, err := s.Decode(b)
	if err != nil || list == nil {
		klog.Errorf("could not decode response %s in saveListObject, response content type: %s, requestInfo: %s, %v",
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
		klog.Errorf("could not update the DynamicRESTMapper %v", err)
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
			Group:     info.APIGroup,
			Version:   info.APIVersion,
		})
		return cm.storeObjectWithKey(key, items[0])
	} else {
		// list all objects or with fieldselector/labelselector
		objs := make(map[storage.Key]runtime.Object)
		comp, _ := util.ClientComponentFrom(ctx)
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
				Group:     info.APIGroup,
				Version:   info.APIVersion,
			})
			objs[key] = items[i]
		}
		// if no objects in cloud cluster(objs is empty), it will clean the old files in the path of rootkey
		return cm.storage.ReplaceComponentList(comp, schema.GroupVersionResource{
			Group:    info.APIGroup,
			Version:  info.APIVersion,
			Resource: info.Resource,
		}, info.Namespace, objs)
	}
}

func (cm *cacheManager) saveOneObject(ctx context.Context, info *apirequest.RequestInfo, b []byte) error {
	comp, _ := util.ClientComponentFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)

	s := cm.serializerManager.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
	if s == nil {
		klog.Errorf("could not create serializer in saveOneObject, %s", util.ReqInfoString(info))
		return fmt.Errorf("could not create serializer in saveOneObject, %s", util.ReqInfoString(info))
	}

	obj, err := s.Decode(b)
	if err != nil {
		klog.Errorf("could not decode response %s in saveOneObject(respContentType:%s): %s, %v", string(b), respContentType, util.ReqInfoString(info), err)
		return err
	} else if obj == nil {
		klog.Info("could not decode nil object. skip cache")
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
		Group:     info.APIGroup,
		Version:   info.APIVersion,
	})
	if err != nil {
		klog.Errorf("could not get cache key(%s:%s:%s:%s), %v", comp, info.Resource, info.Namespace, info.Name, err)
		return err
	}

	// Verify if DynamicRESTMapper(which store the CRD info) needs to be updated
	gvk := obj.GetObjectKind().GroupVersionKind()
	if err := cm.restMapperManager.UpdateKind(gvk); err != nil {
		klog.Errorf("could not update the DynamicRESTMapper %v", err)
	}

	err = cm.storeObjectWithKey(key, obj)
	if err != nil {
		klog.Errorf("could not store object %s, %v", key.Key(), err)
		return err
	}
	return cm.updateInMemoryCache(ctx, info, obj)
}

func (cm *cacheManager) updateInMemoryCache(ctx context.Context, info *apirequest.RequestInfo, obj runtime.Object) error {
	// update the in-memory cache with cloud response
	if !isInMemoryCache(ctx) {
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

func (cm *cacheManager) storeObjectWithKey(key storage.Key, obj runtime.Object) error {
	accessor := meta.NewAccessor()
	if isNotAssignedPod(obj) {
		ns, _ := accessor.Namespace(obj)
		name, _ := accessor.Name(obj)
		return fmt.Errorf("pod(%s/%s) is not assigned to a node, skip cache it", ns, name)
	}

	newRv, err := accessor.ResourceVersion(obj)
	if err != nil {
		return fmt.Errorf("could not get new object resource version for %s, %v", key.Key(), err)
	}

	klog.V(4).Infof("try to store obj of key %s, obj: %v", key.Key(), obj)
	newRvUint, _ := strconv.ParseUint(newRv, 10, 64)
	_, err = cm.storage.Update(key, obj, newRvUint)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrStorageNotFound):
		klog.V(4).Infof("find no cached obj of key: %s, create it with the coming obj with rv: %s", key.Key(), newRv)
		if err := cm.storage.Create(key, obj); err != nil {
			if errors.Is(err, storage.ErrStorageAccessConflict) {
				klog.V(2).Infof("skip to cache obj because key(%s) is under processing", key.Key())
				return nil
			}
			return fmt.Errorf("could not create obj of key: %s, %v", key.Key(), err)
		}
	case errors.Is(err, storage.ErrStorageAccessConflict):
		klog.V(2).Infof("skip to cache watch event because key(%s) is under processing", key.Key())
		return nil
	default:
		return fmt.Errorf("could not store obj with rv %s of key: %s, %v", newRv, key.Key(), err)
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
// 4. csr and sar resource request
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

	if _, ok := nonCacheableResources[info.Resource]; ok {
		return false
	}

	cm.Lock()
	defer cm.Unlock()
	if info.Verb == "list" && info.Name == "" {
		key, err := cm.storage.KeyFunc(storage.KeyBuildInfo{
			Component: comp,
			Resources: info.Resource,
			Namespace: info.Namespace,
			Name:      info.Name,
			Group:     info.APIGroup,
			Version:   info.APIVersion,
		})
		if err != nil {
			return false
		}
		selector, _ := util.ListSelectorFrom(ctx)
		if oldSelector, ok := cm.listSelectorCollector[key]; ok {
			if oldSelector != selector {
				// list requests that have the same path but with different selector, for example:
				// request1: http://{ip:port}/api/v1/default/pods?labelSelector=foo=bar
				// request2: http://{ip:port}/api/v1/default/pods?labelSelector=foo2=bar2
				// because func queryListObject() will get all pods for both requests instead of
				// getting pods by request selector. so cache manager can not support same path list
				// requests that has different selector.
				klog.Warningf("list requests that have the same path but with different selector, skip cache for %s", util.ReqString(req))
				return false
			}
		} else {
			// list requests that get the same resources but with different path, for example:
			// request1: http://{ip/port}/api/v1/pods?fieldSelector=spec.nodeName=foo
			// request2: http://{ip/port}/api/v1/default/pods?fieldSelector=spec.nodeName=foo
			// because func queryListObject() will get all pods for both requests instead of
			// getting pods by request selector. so cache manager can not support getting same resource
			// list requests that has different path.
			for k := range cm.listSelectorCollector {
				if (len(k.Key()) > len(key.Key()) && strings.Contains(k.Key(), key.Key())) || (len(k.Key()) < len(key.Key()) && strings.Contains(key.Key(), k.Key())) {
					klog.Warningf("list requests that get the same resources but with different path, skip cache for %s", util.ReqString(req))
					return false
				}
			}
			cm.listSelectorCollector[key] = selector
		}
	}

	return true
}

// DeleteKindFor is used to delete the invalid Kind(which is not registered in the cloud)
func (cm *cacheManager) DeleteKindFor(gvr schema.GroupVersionResource) error {
	return cm.restMapperManager.DeleteKindFor(gvr)
}

func (cm *cacheManager) queryInMemeryCache(ctx context.Context, reqInfo *apirequest.RequestInfo) (runtime.Object, error) {
	if !isInMemoryCache(ctx) {
		return nil, ErrNotNodeOrLease
	}

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

// isInMemoryCache verify if the response of the request should be cached in-memory.
// In order to accelerate kubelet get node and lease object, we cache them
func isInMemoryCache(reqCtx context.Context) bool {
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

// isListRequestWithNameFieldSelector will check if the request has FieldSelector "metadata.name".
// If found, return true, otherwise false.
func isListRequestWithNameFieldSelector(req *http.Request) bool {
	ctx := req.Context()
	if info, ok := apirequest.RequestInfoFrom(ctx); ok {
		if info.IsResourceRequest && info.Verb == "list" {
			opts := metainternalversion.ListOptions{}
			if err := metainternalversionscheme.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &opts); err == nil {
				if opts.FieldSelector == nil {
					return false
				}
				if _, found := opts.FieldSelector.RequiresExactMatch("metadata.name"); found {
					return true
				}
			}
		}
	}
	return false
}
