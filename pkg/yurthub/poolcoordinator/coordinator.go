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
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	yurtrest "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/certmanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/constants"
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

// Coordinator will track the status of pool coordinator, and change the
// cache and proxy behaviour of yurthub accordingly.
type Coordinator interface {
	// Start the Coordinator.
	Run()
	// IsReady will return the poolCacheManager and true if the pool-coordinator is ready.
	// Pool-Coordinator ready means it is ready to handle request. To be specific, it should
	// satisfy the following 3 condition:
	// 1. Pool-Coordinator is healthy
	// 2. Pool-Scoped resources have been synced with cloud, through list/watch
	// 3. local cache has been uploaded to pool-coordinator
	IsReady() (cachemanager.CacheManager, bool)
	// IsCoordinatorHealthy will return the poolCacheManager and true if the pool-coordinator is healthy.
	// We assume coordinator is healthy when the elect status is LeaderHub and FollowerHub.
	IsHealthy() (cachemanager.CacheManager, bool)
}

type coordinator struct {
	sync.Mutex
	ctx                  context.Context
	cancelEtcdStorage    func()
	informerFactory      informers.SharedInformerFactory
	restMapperMgr        *meta.RESTMapperManager
	serializerMgr        *serializer.SerializerManager
	restConfigMgr        *yurtrest.RestConfigManager
	etcdStorageCfg       *etcd.EtcdStorageConfig
	poolCacheManager     cachemanager.CacheManager
	diskStorage          storage.Store
	etcdStorage          storage.Store
	hubElector           *HubElector
	electStatus          int32
	isPoolCacheSynced    bool
	certMgr              *certmanager.CertManager
	needUploadLocalCache bool
	// poolScopeCacheSyncManager is used to sync pool-scoped resources from cloud to poolcoordinator.
	poolScopeCacheSyncManager *poolScopedCacheSyncManager
	// informerSyncLeaseManager is used to detect the leader-informer-sync lease
	// to check its RenewTime. If its renewTime is not updated after defaultInformerLeaseRenewDuration
	// we can think that the poolcoordinator cache is stale and the poolcoordinator is not ready.
	// It will start if yurthub becomes leader or follower.
	informerSyncLeaseManager *coordinatorLeaseInformerManager
	// delegateNodeLeaseManager is used to list/watch kube-node-lease from poolcoordinator. If the
	// node lease contains DelegateHeartBeat label, it will triger the eventhandler which will
	// use cloud client to send it to cloud APIServer.
	delegateNodeLeaseManager *coordinatorLeaseInformerManager
}

func NewCoordinator(
	ctx context.Context,
	cfg *config.YurtHubConfiguration,
	restMgr *yurtrest.RestConfigManager,
	certMgr *certmanager.CertManager,
	coordinatorTransMgr transport.Interface,
	elector *HubElector) (*coordinator, error) {
	etcdStorageCfg := &etcd.EtcdStorageConfig{
		Prefix:        cfg.CoordinatorStoragePrefix,
		EtcdEndpoints: []string{cfg.CoordinatorStorageAddr},
		CaFile:        certMgr.GetCaFile(),
		CertFile:      certMgr.GetFilePath(certmanager.YurthubClientCert),
		KeyFile:       certMgr.GetFilePath(certmanager.YurthubClientKey),
		LocalCacheDir: cfg.DiskCachePath,
	}

	coordinatorRESTCfg := &rest.Config{
		Host:      cfg.CoordinatorServerURL.String(),
		Transport: coordinatorTransMgr.CurrentTransport(),
		Timeout:   defaultInformerLeaseRenewDuration,
	}
	coordinatorClient, err := kubernetes.NewForConfig(coordinatorRESTCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for pool coordinator, %v", err)
	}

	coordinator := &coordinator{
		ctx:             ctx,
		etcdStorageCfg:  etcdStorageCfg,
		restConfigMgr:   restMgr,
		certMgr:         certMgr,
		informerFactory: cfg.SharedFactory,
		diskStorage:     cfg.StorageWrapper.GetStorage(),
		serializerMgr:   cfg.SerializerManager,
		restMapperMgr:   cfg.RESTMapperManager,
		hubElector:      elector,
	}

	informerSyncLeaseManager := &coordinatorLeaseInformerManager{
		ctx:               ctx,
		coordinatorClient: coordinatorClient,
	}

	delegateNodeLeaseManager := &coordinatorLeaseInformerManager{
		ctx:               ctx,
		coordinatorClient: coordinatorClient,
	}

	proxiedClient, err := buildProxiedClientWithUserAgent(fmt.Sprintf("http://%s", cfg.YurtHubProxyServerAddr), constants.DefaultPoolScopedUserAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxied client, %v", err)
	}
	poolScopedCacheSyncManager := &poolScopedCacheSyncManager{
		ctx:               ctx,
		proxiedClient:     proxiedClient,
		coordinatorClient: cfg.CoordinatorClient,
		nodeName:          cfg.NodeName,
		getEtcdStore:      coordinator.getEtcdStore,
	}

	coordinator.informerSyncLeaseManager = informerSyncLeaseManager
	coordinator.delegateNodeLeaseManager = delegateNodeLeaseManager
	coordinator.poolScopeCacheSyncManager = poolScopedCacheSyncManager

	return coordinator, nil
}

func (coordinator *coordinator) Run() {
	for {
		var poolCacheManager cachemanager.CacheManager
		var cancelEtcdStorage func()
		var needUploadLocalCache bool
		var needCancelEtcdStorage bool
		var isPoolCacheSynced bool
		var etcdStorage storage.Store
		var err error

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
				needUploadLocalCache = true
				needCancelEtcdStorage = true
				isPoolCacheSynced = false
				etcdStorage = nil
				poolCacheManager = nil
			case LeaderHub:
				poolCacheManager, etcdStorage, cancelEtcdStorage, err = coordinator.buildPoolCacheStore()
				if err != nil {
					klog.Errorf("failed to create pool scoped cache store and manager, %v", err)
					continue
				}

				cloudLeaseClient, err := coordinator.newCloudLeaseClient()
				if err != nil {
					klog.Errorf("cloud not get cloud lease client when becoming leader yurthub, %v", err)
					continue
				}
				if err := coordinator.poolScopeCacheSyncManager.EnsureStart(); err != nil {
					klog.Errorf("failed to sync pool-scoped resource, %v", err)
					continue
				}

				coordinator.delegateNodeLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
					FilterFunc: ifDelegateHeartBeat,
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: func(obj interface{}) {
							coordinator.delegateNodeLease(cloudLeaseClient, obj)
						},
						UpdateFunc: func(_, newObj interface{}) {
							coordinator.delegateNodeLease(cloudLeaseClient, newObj)
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
							coordinator.Lock()
							defer coordinator.Unlock()
							coordinator.isPoolCacheSynced = false
						},
					},
				})

				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(etcdStorage); err != nil {
						klog.Errorf("failed to upload local cache when yurthub becomes leader, %v", err)
					} else {
						needUploadLocalCache = false
					}
				}
			case FollowerHub:
				poolCacheManager, etcdStorage, cancelEtcdStorage, err = coordinator.buildPoolCacheStore()
				if err != nil {
					klog.Errorf("failed to create pool scoped cache store and manager, %v", err)
					continue
				}

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
							coordinator.Lock()
							defer coordinator.Unlock()
							coordinator.isPoolCacheSynced = false
						},
					},
				})

				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(etcdStorage); err != nil {
						klog.Errorf("failed to upload local cache when yurthub becomes follower, %v", err)
					} else {
						needUploadLocalCache = false
					}
				}
			}

			// We should make sure that all fields update should happen
			// after acquire lock to avoid race condition.
			// Because the caller of IsReady() may be concurrent.
			coordinator.Lock()
			if needCancelEtcdStorage {
				coordinator.cancelEtcdStorage()
			}
			coordinator.electStatus = electorStatus
			coordinator.poolCacheManager = poolCacheManager
			coordinator.etcdStorage = etcdStorage
			coordinator.cancelEtcdStorage = cancelEtcdStorage
			coordinator.needUploadLocalCache = needUploadLocalCache
			coordinator.isPoolCacheSynced = isPoolCacheSynced
			coordinator.Unlock()
		}
	}
}

// IsReady will return the poolCacheManager and true if the pool-coordinator is ready.
// Pool-Coordinator ready means it is ready to handle request. To be specific, it should
// satisfy the following 3 condition:
// 1. Pool-Coordinator is healthy
// 2. Pool-Scoped resources have been synced with cloud, through list/watch
// 3. local cache has been uploaded to pool-coordinator
func (coordinator *coordinator) IsReady() (cachemanager.CacheManager, bool) {
	// If electStatus is not PendingHub, it means pool-coordinator is healthy.
	coordinator.Lock()
	defer coordinator.Unlock()
	if coordinator.electStatus != PendingHub && coordinator.isPoolCacheSynced && !coordinator.needUploadLocalCache {
		return coordinator.poolCacheManager, true
	}
	return nil, false
}

// IsCoordinatorHealthy will return the poolCacheManager and true if the pool-coordinator is healthy.
// We assume coordinator is healthy when the elect status is LeaderHub and FollowerHub.
func (coordinator *coordinator) IsHealthy() (cachemanager.CacheManager, bool) {
	coordinator.Lock()
	defer coordinator.Unlock()
	if coordinator.electStatus != PendingHub {
		return coordinator.poolCacheManager, true
	}
	return nil, false
}

func (coordinator *coordinator) buildPoolCacheStore() (cachemanager.CacheManager, storage.Store, func(), error) {
	ctx, cancel := context.WithCancel(coordinator.ctx)
	etcdStore, err := etcd.NewStorage(ctx, coordinator.etcdStorageCfg)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("failed to create etcd storage, %v", err)
	}
	poolCacheManager := cachemanager.NewCacheManager(
		cachemanager.NewStorageWrapper(etcdStore),
		coordinator.serializerMgr,
		coordinator.restMapperMgr,
		coordinator.informerFactory,
	)
	return poolCacheManager, etcdStore, cancel, nil
}

func (coordinator *coordinator) getEtcdStore() storage.Store {
	return coordinator.etcdStorage
}

func (coordinator *coordinator) newCloudLeaseClient() (coordclientset.LeaseInterface, error) {
	restCfg := coordinator.restConfigMgr.GetRestConfig(true)
	if restCfg == nil {
		return nil, fmt.Errorf("no cloud server is healthy")
	}
	cloudClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud client, %v", err)
	}

	return cloudClient.CoordinationV1().Leases(corev1.NamespaceNodeLease), nil
}

func (coordinator *coordinator) uploadLocalCache(etcdStore storage.Store) error {
	uploader := &localCacheUploader{
		diskStorage: coordinator.diskStorage,
		etcdStorage: etcdStore,
	}
	klog.Info("uploading local cache")
	uploader.Upload()
	return nil
}

func (coordinator *coordinator) delegateNodeLease(cloudLeaseClient coordclientset.LeaseInterface, obj interface{}) {
	newLease := obj.(*coordinationv1.Lease)
	for i := 0; i < leaseDelegateRetryTimes; i++ {
		// ResourceVersions of lease objects in pool-coordinator always have different rv
		// from what of cloud lease. So we should get cloud lease first and then update
		// it with lease from pool-coordinator.
		cloudLease, err := cloudLeaseClient.Get(coordinator.ctx, newLease.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := cloudLeaseClient.Create(coordinator.ctx, cloudLease, metav1.CreateOptions{}); err != nil {
				klog.Errorf("failed to create lease %s at cloud, %v", newLease.Name, err)
				continue
			}
		}

		lease := newLease.DeepCopy()
		lease.ResourceVersion = cloudLease.ResourceVersion
		if _, err := cloudLeaseClient.Update(coordinator.ctx, lease, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("failed to update lease %s at cloud, %v", newLease.Name, err)
			continue
		}
	}
}

func (coordinator *coordinator) detectPoolCacheSynced(obj interface{}) {
	lease := obj.(*coordinationv1.Lease)
	renewTime := lease.Spec.RenewTime
	if time.Now().After(renewTime.Add(defaultPoolCacheStaleDuration)) {
		coordinator.Lock()
		defer coordinator.Unlock()
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
	getEtcdStore        func() storage.Store
	cancel              func()
}

func (p *poolScopedCacheSyncManager) EnsureStart() error {
	if !p.isRunning {
		if err := p.coordinatorClient.CoordinationV1().Leases(namespaceInformerLease).Delete(p.ctx, nameInformerLease, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete informer sync lease, %v", err)
		}

		etcdStore := p.getEtcdStore()
		if etcdStore == nil {
			return fmt.Errorf("got empty etcd storage")
		}
		if err := etcdStore.DeleteComponentResources(constants.DefaultPoolScopedUserAgent); err != nil {
			return fmt.Errorf("failed to clean old pool-scoped cache, %v", err)
		}

		ctx, cancel := context.WithCancel(p.ctx)
		hasInformersSynced := []cache.InformerSynced{}
		informerFactory := informers.NewSharedInformerFactory(p.proxiedClient, 0)
		for gvr := range constants.PoolScopedResources {
			informer, err := informerFactory.ForResource(gvr)
			if err != nil {
				cancel()
				return fmt.Errorf("failed to add informer for %s, %v", gvr.String(), err)
			}
			hasInformersSynced = append(hasInformersSynced, informer.Informer().HasSynced)
		}

		informerFactory.Start(ctx.Done())
		go p.holdInformerSync(ctx, hasInformersSynced)
		p.cancel = cancel
		p.isRunning = true
	}
	return nil
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
	for info := range constants.UploadResourcesKeyBuildInfo {
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

func buildProxiedClientWithUserAgent(proxyAddr string, userAgent string) (kubernetes.Interface, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(proxyAddr, "")
	if err != nil {
		return nil, err
	}

	kubeConfig.UserAgent = userAgent
	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}
