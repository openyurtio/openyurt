package cachemanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/alibaba/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"github.com/alibaba/openyurt/pkg/yurthub/util"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
)

type CacheManager interface {
	CacheResponse(ctx context.Context, prc io.ReadCloser, stopCh <-chan struct{}) error
	QueryCache(req *http.Request) (runtime.Object, error)
	UpdateCacheAgents(agents []string) error
	ListCacheAgents() []string
	CanCacheFor(req *http.Request) bool
}

type cacheManager struct {
	sync.RWMutex
	storage           StorageWrapper
	serializerManager *serializer.SerializerManager
	cacheAgents       map[string]bool
}

func NewCacheManager(
	storage StorageWrapper,
	serializerMgr *serializer.SerializerManager,
) (CacheManager, error) {
	cm := &cacheManager{
		storage:           storage,
		serializerManager: serializerMgr,
		cacheAgents:       make(map[string]bool),
	}

	err := cm.initCacheAgents()
	if err != nil {
		return nil, err
	}

	return cm, nil
}

func (em *cacheManager) CacheResponse(ctx context.Context, prc io.ReadCloser, stopCh <-chan struct{}) error {
	info, _ := apirequest.RequestInfoFrom(ctx)
	if isWatch(ctx) {
		return em.saveWatchObject(ctx, info, prc, stopCh)
	} else {
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
			return em.saveListObject(ctx, info, buf.Bytes())
		} else {
			return em.saveOneObject(ctx, info, buf.Bytes())
		}
	}
}

func (em *cacheManager) QueryCache(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	info, ok := apirequest.RequestInfoFrom(ctx)
	if !ok || info == nil || info.Resource == "" {
		return nil, fmt.Errorf("failed to get request info")
	}

	comp, ok := util.ClientComponentFrom(ctx)
	if !ok || comp == "" {
		return nil, fmt.Errorf("failed to get component info")
	}

	if info.IsResourceRequest && info.Verb == "list" {
		return em.queryListObject(req)
	} else if info.IsResourceRequest && (info.Verb == "get" || info.Verb == "patch" || info.Verb == "update") {
		key, err := util.KeyFunc(comp, info.Resource, info.Namespace, info.Name)
		if err != nil {
			return nil, err
		}
		return em.storage.Get(key)
	}

	return nil, fmt.Errorf("request(%#+v) is not supported", info)
}

func (em *cacheManager) queryListObject(req *http.Request) (runtime.Object, error) {
	ctx := req.Context()
	comp, _ := util.ClientComponentFrom(ctx)
	info, _ := apirequest.RequestInfoFrom(ctx)

	listKind := ResourceToListKindMap[info.Resource]
	listGvk := schema.GroupVersionKind{
		Group:   info.APIGroup,
		Version: info.APIVersion,
		Kind:    listKind,
	}

	listObj, err := scheme.Scheme.New(listGvk)
	if err != nil {
		klog.Errorf("failed to create list object(%v), %v", listGvk, err)
		return nil, err
	}

	key, err := util.KeyFunc(comp, info.Resource, info.Namespace, info.Name)
	if err != nil {
		return nil, err
	}

	objs, err := em.storage.List(key)
	if err != nil {
		return nil, err
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

	prefix := "/" + path.Join(info.APIGroup, info.APIGroup)
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

func (em *cacheManager) saveWatchObject(ctx context.Context, info *apirequest.RequestInfo, r io.ReadCloser, stopCh <-chan struct{}) error {
	delObjCnt := 0
	updateObjCnt := 0
	addObjCnt := 0

	comp, _ := util.ClientComponentFrom(ctx)
	reqContentType, _ := util.ReqContentTypeFrom(ctx)
	serializers, err := em.serializerManager.CreateSerializers(reqContentType, info.APIGroup, info.APIVersion)
	if err != nil {
		klog.Errorf("failed to create serializers in saveWatchObject, %v", err)
		return err
	}

	kind := ResourceToKindMap[info.Resource]
	apiVersion := schema.GroupVersion{
		Group:   info.APIGroup,
		Version: info.APIVersion,
	}.String()
	accessor := meta.NewAccessor()

	d, err := serializer.WatchDecoder(serializers, r)
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

			key, err := util.KeyFunc(comp, info.Resource, ns, name)
			if err != nil || key == "" {
				klog.Errorf("failed to get cache path, %v", err)
				continue
			}

			switch watchType {
			case watch.Added, watch.Modified:
				accessor.SetAPIVersion(obj, apiVersion)
				accessor.SetKind(obj, kind)
				err = em.saveOneObjectWithValidation(key, obj)
				if watchType == watch.Added {
					addObjCnt++
				} else {
					updateObjCnt++
				}
			case watch.Deleted:
				err = em.storage.Delete(key)
				delObjCnt++
			default:
				// impossible go to here
			}

			if info.Resource == "pods" {
				klog.V(2).Infof("pod(%s) is %s", key, string(watchType))
			}

			if err == storage.ErrStorageAccessConflict {
				klog.V(2).Infof("skip to cache watch event because key(%s) is under processing", key)
			} else if err != nil {
				klog.Errorf("failed to process watch object %s, %v", key, err)
			}
		case watch.Error:
			klog.Infof("unable to understand watch event %#v", obj)
		}
	}
}

func (em *cacheManager) saveListObject(ctx context.Context, info *apirequest.RequestInfo, b []byte) error {
	reqContentType, _ := util.ReqContentTypeFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	serializers, err := em.serializerManager.CreateSerializers(reqContentType, info.APIGroup, info.APIVersion)
	if err != nil {
		klog.Errorf("failed to create serializers in saveListObject, %v", err)
		return err
	}

	list, err := serializer.DecodeResp(serializers, b, reqContentType, respContentType)
	if err != nil {
		klog.Errorf("failed to decode response in saveOneObject %v", err)
		return err
	} else {
		switch list.(type) {
		case *metav1.Status:
			// it's not need to cache for status
			klog.Infof("it's not need to cache metav1.Status")
			return nil
		}
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		klog.Errorf("unable to understand list result %#v (%v)", list, err)
		return fmt.Errorf("unable to understand list result %#v (%v)", list, err)
	}
	klog.V(5).Infof("list items for %s is: %d", util.ReqInfoString(info), len(items))

	kind := ResourceToKindMap[info.Resource]
	apiVersion := schema.GroupVersion{
		Group:   info.APIGroup,
		Version: info.APIVersion,
	}.String()
	accessor := meta.NewAccessor()

	comp, _ := util.ClientComponentFrom(ctx)
	var errs []error
	for i := range items {
		name, err := accessor.Name(items[i])
		if err != nil || name == "" {
			klog.Errorf("failed to get name of list items object, %v", err)
			continue
		}

		ns, err := accessor.Namespace(items[i])
		if err != nil {
			klog.Errorf("failed to get namespace of list items object, %v", err)
			continue
		} else if ns == "" {
			ns = info.Namespace
		}

		klog.V(5).Infof("path for list item(%d): %s/%s/%s/%s", i, comp, info.Resource, ns, name)
		key, err := util.KeyFunc(comp, info.Resource, ns, name)
		if err != nil || key == "" {
			klog.Errorf("failed to get cache key(%s:%s:%s:%s), %v", comp, info.Resource, ns, name, err)
			return err
		}

		accessor.SetKind(items[i], kind)
		accessor.SetAPIVersion(items[i], apiVersion)
		err = em.saveOneObjectWithValidation(key, items[i])
		if err == storage.ErrStorageAccessConflict {
			klog.V(2).Infof("skip to cache list object because key(%s) is under processing", key)
		} else if err != nil {
			errs = append(errs, fmt.Errorf("failed to save object(%s), %v", key, err))
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("failed to save list object, %#+v", errs)
	}

	return nil
}

func (em *cacheManager) saveOneObject(ctx context.Context, info *apirequest.RequestInfo, b []byte) error {
	comp, _ := util.ClientComponentFrom(ctx)
	reqContentType, _ := util.ReqContentTypeFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)

	serializers, err := em.serializerManager.CreateSerializers(reqContentType, info.APIGroup, info.APIVersion)
	if err != nil {
		klog.Errorf("failed to create serializers in saveOneObject: %s, %v", util.ReqInfoString(info), err)
		return err
	}

	accessor := meta.NewAccessor()
	obj, err := serializer.DecodeResp(serializers, b, reqContentType, respContentType)
	if err != nil {
		klog.Errorf("failed to decode response in saveOneObject(reqContentType:%s, respContentType:%s): %s, %v", reqContentType, respContentType, util.ReqInfoString(info), err)
		return err
	} else if obj == nil {
		klog.Infof("it's not need to cache metav1.Status.")
		return nil
	} else {
		switch obj.(type) {
		case *metav1.Status:
			// it's not need to cache for status
			return nil
		}

		kind := ResourceToKindMap[info.Resource]
		apiVersion := schema.GroupVersion{
			Group:   info.APIGroup,
			Version: info.APIVersion,
		}.String()

		accessor.SetKind(obj, kind)
		accessor.SetAPIVersion(obj, apiVersion)
	}

	var name string
	if isCreate(ctx) {
		name, _ = accessor.Name(obj)
	} else {
		name = info.Name
	}

	if name == "" {
		klog.Errorf("cache object have no name, %s", info.Path)
		return nil
	}

	key, err := util.KeyFunc(comp, info.Resource, info.Namespace, name)
	if err != nil || key == "" {
		klog.Errorf("failed to get cache key(%s:%s:%s:%s), %v", comp, info.Resource, info.Namespace, info.Name, err)
		return err
	}

	if err := em.saveOneObjectWithValidation(key, obj); err != nil {
		if err != storage.ErrStorageAccessConflict {
			return err
		}
		klog.V(2).Infof("skip to cache object because key(%s) is under processing", key)
	}

	return nil
}

func (em *cacheManager) saveOneObjectWithValidation(key string, obj runtime.Object) error {
	oldObj, err := em.storage.Get(key)
	if err == nil && oldObj != nil {
		accessor := meta.NewAccessor()

		oldRv, err := accessor.ResourceVersion(oldObj)
		if err != nil {
			klog.Errorf("failed to get old object resource version for %s, %v", key, err)
			return err
		}

		newRv, err := accessor.ResourceVersion(obj)
		if err != nil {
			klog.Errorf("failed to get new object resource version for %s, %v", key, err)
			return err
		}

		oldRvInt, _ := strconv.Atoi(oldRv)
		newRvInt, _ := strconv.Atoi(newRv)
		if newRvInt <= oldRvInt { // resource version is incremented or not
			return nil
		}

		return em.storage.Update(key, obj)
	} else if os.IsNotExist(err) || oldObj == nil {
		return em.storage.Create(key, obj)
	} else {
		if err != storage.ErrStorageAccessConflict {
			return em.storage.Create(key, obj)
		}
		return err
	}
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
