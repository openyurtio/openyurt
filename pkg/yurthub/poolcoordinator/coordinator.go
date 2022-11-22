/*
Copyright 2022 The OpenYurt Authors.

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

package poolcoordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	coordclientset "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/etcd"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
)

const (
	leaseDelegateRetryTimes           = 5
	defaultInformerLeaseRenewDuration = 10 * time.Second
	defaultPoolCacheStaleDuration     = 30 * time.Second
	namespaceInformerLease            = "kube-system"
	nameInformerLease                 = "leader-informer-sync"
)

type Coordinator struct {
	sync.Mutex
	ctx                       context.Context
	cancelEtcdStorage         func()
	informerFactory           informers.SharedInformerFactory
	restMapperMgr             *meta.RESTMapperManager
	serializerMgr             *serializer.SerializerManager
	etcdStorageCfg            *etcd.EtcdStorageConfig
	poolCacheManager          cachemanager.CacheManager
	localCacheUploader        *localCacheUploader
	cloudLeaseClient          coordclientset.LeaseInterface
	hubElector                *HubElector
	electStatus               int32
	isPoolCacheSynced         bool
	needUploadLocalCache      bool
	poolScopeCacheSyncManager *poolScopedCacheSyncManager
	informerSyncLeaseManager  *coordinatorLeaseInformerManager
	delegateNodeLeaseManager  *coordinatorLeaseInformerManager
}

func NewCoordinator(
	ctx context.Context,
	cfg *config.YurtHubConfiguration,
	kubeClient kubernetes.Interface,
	transportMgr transport.Interface,
	elector *HubElector) (*Coordinator, error) {
	uploader := &localCacheUploader{
		diskStorage: cfg.StorageWrapper.GetStorage(),
	}

	etcdStorageCfg := &etcd.EtcdStorageConfig{
		EtcdEndpoints: []string{cfg.CoordinatorStorageAddr},
		CaFile:        cfg.CoordinatorStorageCaFile,
		CertFile:      cfg.CoordinatorStorageCertFile,
		KeyFile:       cfg.CoordinatorStorageKeyFile,
	}

	coordinatorRESTCfg := &rest.Config{
		Host:      cfg.CoordinatorServer.String(),
		Transport: transportMgr.CurrentTransport(),
		Timeout:   defaultInformerLeaseRenewDuration,
	}
	coordinatorClient, err := kubernetes.NewForConfig(coordinatorRESTCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for pool coordinator, %v", err)
	}

	coordinator := &Coordinator{
		ctx:                ctx,
		etcdStorageCfg:     etcdStorageCfg,
		cloudLeaseClient:   kubeClient.CoordinationV1().Leases(corev1.NamespaceNodeLease),
		localCacheUploader: uploader,
		informerFactory:    cfg.SharedFactory,
		serializerMgr:      cfg.SerializerManager,
		restMapperMgr:      cfg.RESTMapperManager,
		hubElector:         elector,
	}

	informerSyncLeaseManager := &coordinatorLeaseInformerManager{
		ctx:               ctx,
		coordinatorClient: coordinatorClient,
	}

	delegateNodeLeaseManager := &coordinatorLeaseInformerManager{
		ctx:               ctx,
		coordinatorClient: coordinatorClient,
	}

	poolScopedCacheSyncManager := &poolScopedCacheSyncManager{
		ctx:               ctx,
		proxiedClient:     cfg.ProxiedClient,
		coordinatorClient: cfg.CoordinatorClient,
		nodeName:          cfg.NodeName,
	}

	coordinator.informerSyncLeaseManager = informerSyncLeaseManager
	coordinator.delegateNodeLeaseManager = delegateNodeLeaseManager
	coordinator.poolScopeCacheSyncManager = poolScopedCacheSyncManager

	return coordinator, nil
}

func (coordinator *Coordinator) Run() {
	for {
		select {
		case <-coordinator.ctx.Done():
			coordinator.poolScopeCacheSyncManager.EnsureStop()
			coordinator.delegateNodeLeaseManager.EnsureStop()
			coordinator.informerSyncLeaseManager.EnsureStop()
			klog.Info("exit normally in coordinator loop.")
			return
		case electorStatus, ok := <-coordinator.hubElector.StatusChan():
			if !ok {
				return
			}

			switch electorStatus {
			case PendingHub:
				coordinator.poolScopeCacheSyncManager.EnsureStop()
				coordinator.delegateNodeLeaseManager.EnsureStop()
				coordinator.informerSyncLeaseManager.EnsureStop()
				coordinator.stopPoolCacheManager()
				coordinator.needUploadLocalCache = true
			case LeaderHub:
				coordinator.poolScopeCacheSyncManager.EnsureStart()
				coordinator.delegateNodeLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
					FilterFunc: ifDelegateHeartBeat,
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: coordinator.delegateNodeLease,
						UpdateFunc: func(_, newObj interface{}) {
							coordinator.delegateNodeLease(newObj)
						},
					},
				})
				coordinator.informerSyncLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
					FilterFunc: ifInformerSyncLease,
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: coordinator.detectPoolCacheSynced,
						UpdateFunc: func(_, newObj interface{}) {
							coordinator.detectPoolCacheSynced(newObj)
						},
						DeleteFunc: func(_ interface{}) {
							coordinator.isPoolCacheSynced = false
						},
					},
				})
				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(); err != nil {
						klog.Errorf("failed to upload local cache when yurthub becomes leader, %v", err)
						continue
					}
					coordinator.needUploadLocalCache = false
				}
			case FollowerHub:
				coordinator.poolScopeCacheSyncManager.EnsureStop()
				coordinator.delegateNodeLeaseManager.EnsureStop()
				coordinator.informerSyncLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
					FilterFunc: ifInformerSyncLease,
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: coordinator.detectPoolCacheSynced,
						UpdateFunc: func(_, newObj interface{}) {
							coordinator.detectPoolCacheSynced(newObj)
						},
						DeleteFunc: func(_ interface{}) {
							coordinator.isPoolCacheSynced = false
						},
					},
				})
				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(); err != nil {
						klog.Errorf("failed to upload local cache when yurthub becomes follower, %v", err)
						continue
					}
					coordinator.needUploadLocalCache = false
				}
			}
			coordinator.electStatus = electorStatus
		}
	}
}

func (coordinator *Coordinator) IsReady() bool {
	return coordinator.electStatus != PendingHub && coordinator.isPoolCacheSynced
}

func (coordinator *Coordinator) PoolCacheManager() cachemanager.CacheManager {
	return coordinator.poolCacheManager
}

func (coordinator *Coordinator) uploadLocalCache() error {
	ctx, cancel := context.WithCancel(coordinator.ctx)
	etcdStore, err := etcd.NewStorage(ctx, coordinator.etcdStorageCfg)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create etcd storage, %v", err)
	}
	coordinator.localCacheUploader.etcdStorage = etcdStore
	klog.Info("uploading local cache")
	coordinator.localCacheUploader.Upload()

	coordinator.poolCacheManager = cachemanager.NewCacheManager(
		cachemanager.NewStorageWrapper(etcdStore),
		coordinator.serializerMgr,
		coordinator.restMapperMgr,
		coordinator.informerFactory,
	)
	coordinator.cancelEtcdStorage = cancel
	return nil
}

func (coordinator *Coordinator) stopPoolCacheManager() {
	coordinator.cancelEtcdStorage()
	coordinator.poolCacheManager = nil
}

func (coordinator *Coordinator) delegateNodeLease(obj interface{}) {
	newLease := obj.(*coordinationv1.Lease)
	for i := 0; i < leaseDelegateRetryTimes; i++ {
		// ResourceVersions of lease objects in pool-coordinator always have different rv
		// from what of cloud lease. So we should get cloud lease first and then update
		// it with lease from pool-coordinator.
		cloudLease, err := coordinator.cloudLeaseClient.Get(coordinator.ctx, newLease.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := coordinator.cloudLeaseClient.Create(coordinator.ctx, cloudLease, metav1.CreateOptions{}); err != nil {
				klog.Errorf("failed to create lease %s at cloud, %v", newLease.Name, err)
				continue
			}
		}

		lease := newLease.DeepCopy()
		lease.ResourceVersion = cloudLease.ResourceVersion
		if _, err := coordinator.cloudLeaseClient.Update(coordinator.ctx, lease, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("failed to update lease %s at cloud, %v", newLease.Name, err)
			continue
		}
	}
}

func (coordinator *Coordinator) detectPoolCacheSynced(obj interface{}) {
	lease := obj.(*coordinationv1.Lease)
	renewTime := lease.Spec.RenewTime
	if time.Now().After(renewTime.Add(defaultPoolCacheStaleDuration)) {
		coordinator.isPoolCacheSynced = false
	}
}

// poolScopedCacheSyncManager will continuously sync pool-scoped resources from cloud to pool-coordinator.
// After resource sync is completed, it will periodically renew the informer synced lease, which is used by
// other yurthub to determine if pool-coordinator is ready to handle requests of pool-scoped resources.
// It uses proxied client to list/watch pool-scoped resources from cloud APIServer, which
// will be automatically cached into pool-coordinator through YurtProxyServer.
type poolScopedCacheSyncManager struct {
	ctx       context.Context
	isRunning bool
	// proxiedClient is a client of Cloud APIServer which is proxied by yurthub.
	proxiedClient kubernetes.Interface
	// coordinatorClient is a client of APIServer in pool-coordinator.
	coordinatorClient kubernetes.Interface
	// nodeName will be used to update the ownerReference of informer synced lease.
	nodeName            string
	informerSyncedLease *coordinationv1.Lease
	cancel              func()
}

func (p *poolScopedCacheSyncManager) EnsureStart() {
	if !p.isRunning {
		ctx, cancel := context.WithCancel(p.ctx)
		hasInformersSynced := []cache.InformerSynced{}
		informerFactory := informers.NewSharedInformerFactory(p.proxiedClient, 0)
		for gvr := range PoolScopedResources {
			informer, err := informerFactory.ForResource(gvr)
			if err != nil {
				klog.Errorf("failed to add informer for %s, %v", gvr.String(), err)
				continue
			}
			hasInformersSynced = append(hasInformersSynced, informer.Informer().HasSynced)
		}

		informerFactory.Start(ctx.Done())
		go p.holdInformerSync(ctx, hasInformersSynced)
		p.cancel = cancel
		p.isRunning = true
	}
}

func (p *poolScopedCacheSyncManager) EnsureStop() {
	if p.isRunning {
		p.cancel()
		p.cancel = nil
		p.isRunning = false
	}
}

func (p *poolScopedCacheSyncManager) holdInformerSync(ctx context.Context, hasInformersSynced []cache.InformerSynced) {
	if cache.WaitForCacheSync(ctx.Done(), hasInformersSynced...) {
		informerLease := NewInformerLease(
			p.coordinatorClient,
			nameInformerLease,
			namespaceInformerLease,
			p.nodeName,
			int32(defaultInformerLeaseRenewDuration.Seconds()),
			5)
		p.renewInformerLease(ctx, informerLease)
		return
	}
	klog.Error("failed to wait for cache synced, it was canceled")
}

func (p *poolScopedCacheSyncManager) renewInformerLease(ctx context.Context, lease informerLease) {
	for {
		t := time.NewTicker(defaultInformerLeaseRenewDuration)
		select {
		case <-ctx.Done():
			klog.Info("cancel renew informer lease")
			return
		case <-t.C:
			newLease, err := lease.Update(p.informerSyncedLease)
			if err != nil {
				klog.Errorf("failed to update informer lease, %v", err)
				continue
			}
			p.informerSyncedLease = newLease
		}
	}
}

// coordinatorLeaseInformerManager will use pool-coordinator client to list/watch
// lease in pool-coordinator. Through passing different event handler, it can either
// delegating node lease by leader yurthub or detecting the informer synced lease to
// check if pool-coordinator is ready for requests of pool-scoped resources.
type coordinatorLeaseInformerManager struct {
	ctx               context.Context
	coordinatorClient kubernetes.Interface
	name              string
	isRunning         bool
	cancel            func()
}

func (c *coordinatorLeaseInformerManager) Name() string {
	return c.name
}

func (c *coordinatorLeaseInformerManager) EnsureStartWithHandler(handler cache.FilteringResourceEventHandler) {
	if !c.isRunning {
		ctx, cancel := context.WithCancel(c.ctx)
		informerFactory := informers.NewSharedInformerFactory(c.coordinatorClient, 0)
		informerFactory.Coordination().V1().Leases().Informer().AddEventHandler(handler)
		informerFactory.Start(ctx.Done())
		c.isRunning = true
		c.cancel = cancel
	}
}

func (c *coordinatorLeaseInformerManager) EnsureStop() {
	if c.isRunning {
		c.cancel()
		c.isRunning = false
	}
}

// localCacheUploader can upload resources in local cache to pool cache.
// Currently, we only upload pods and nodes to pool-coordinator.
type localCacheUploader struct {
	diskStorage storage.Store
	etcdStorage storage.Store
}

func (l *localCacheUploader) Upload() {
	objBytes := l.resourcesToUpload()
	for k, b := range objBytes {
		rv, err := getRv(b)
		if err != nil {
			klog.Errorf("failed to get name from bytes %s, %v", string(b), err)
			continue
		}

		if err := l.createOrUpdate(k, b, rv); err != nil {
			klog.Errorf("failed to upload %s, %v", k.Key(), err)
		}
	}
}

func (l *localCacheUploader) createOrUpdate(key storage.Key, objBytes []byte, rv uint64) error {
	err := l.etcdStorage.Create(key, objBytes)

	if err == storage.ErrKeyExists {
		// try to update
		_, updateErr := l.etcdStorage.Update(key, objBytes, rv)
		if updateErr == storage.ErrUpdateConflict {
			return nil
		}
		return updateErr
	}

	return err
}

func (l *localCacheUploader) resourcesToUpload() map[storage.Key][]byte {
	objBytes := map[storage.Key][]byte{}
	for info := range UploadResourcesKeyBuildInfo {
		gvr := schema.GroupVersionResource{
			Group:    info.Group,
			Version:  info.Version,
			Resource: info.Resources,
		}
		keys, err := l.diskStorage.ListResourceKeysOfComponent(info.Component, gvr)
		if err != nil {
			klog.Errorf("failed to get object keys from disk for %s, %v", gvr.String(), err)
			continue
		}

		for _, k := range keys {
			buf, err := l.diskStorage.Get(k)
			if err != nil {
				klog.Errorf("failed to read local cache of key %s, %v", k.Key(), err)
				continue
			}
			objBytes[k] = buf
		}
	}
	return objBytes
}

func getRv(objBytes []byte) (uint64, error) {
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(objBytes, obj); err != nil {
		return 0, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	rv, err := strconv.ParseUint(obj.GetResourceVersion(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse rv %s of pod %s, %v", obj.GetName(), obj.GetResourceVersion(), err)
	}

	return rv, nil
}

func ifDelegateHeartBeat(obj interface{}) bool {
	lease, ok := obj.(*coordinationv1.Lease)
	if !ok {
		return false
	}
	v, ok := lease.Labels[healthchecker.DelegateHeartBeat]
	return ok && v == "true"
}

func ifInformerSyncLease(obj interface{}) bool {
	lease, ok := obj.(*coordinationv1.Lease)
	if !ok {
		return false
	}

	return lease.Name == nameInformerLease && lease.Namespace == namespaceInformerLease
}
