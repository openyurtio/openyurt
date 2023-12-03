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

package yurtcoordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
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
	"github.com/openyurtio/openyurt/pkg/yurthub/metrics"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/etcd"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/certmanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/resources"
)

const (
	leaseDelegateRetryTimes           = 5
	defaultInformerLeaseRenewDuration = 10 * time.Second
	defaultPoolCacheStaleDuration     = 30 * time.Second
	namespaceInformerLease            = "kube-system"
	nameInformerLease                 = "leader-informer-sync"
)

// Coordinator will track the status of yurt coordinator, and change the
// cache and proxy behaviour of yurthub accordingly.
type Coordinator interface {
	// Start the Coordinator.
	Run()
	// IsReady will return the poolCacheManager and true if the yurt-coordinator is ready.
	// Yurt-Coordinator ready means it is ready to handle request. To be specific, it should
	// satisfy the following 3 condition:
	// 1. Yurt-Coordinator is healthy
	// 2. Pool-Scoped resources have been synced with cloud, through list/watch
	// 3. local cache has been uploaded to yurt-coordinator
	IsReady() (cachemanager.CacheManager, bool)
	// IsCoordinatorHealthy will return the poolCacheManager and true if the yurt-coordinator is healthy.
	// We assume coordinator is healthy when the elect status is LeaderHub and FollowerHub.
	IsHealthy() (cachemanager.CacheManager, bool)
}

type statusInfo struct {
	electorStatus int32
	currentCnt    uint64
}

type coordinator struct {
	sync.Mutex
	ctx               context.Context
	cancelEtcdStorage func()
	informerFactory   informers.SharedInformerFactory
	restMapperMgr     *meta.RESTMapperManager
	serializerMgr     *serializer.SerializerManager
	restConfigMgr     *yurtrest.RestConfigManager
	etcdStorageCfg    *etcd.EtcdStorageConfig
	poolCacheManager  cachemanager.CacheManager
	diskStorage       storage.Store
	etcdStorage       storage.Store
	hubElector        *HubElector
	electStatus       int32
	cnt               uint64
	statusInfoChan    chan statusInfo
	isPoolCacheSynced bool
	certMgr           *certmanager.CertManager
	// cloudCAFileData is the file data of cloud kubernetes cluster CA cert.
	cloudCAFileData []byte
	// cloudHealthChecker is health checker of cloud APIServers. It is used to
	// pick a healthy cloud APIServer to proxy heartbeats.
	cloudHealthChecker   healthchecker.MultipleBackendsHealthChecker
	needUploadLocalCache bool
	// poolCacheSyncManager is used to sync pool-scoped resources from cloud to yurtcoordinator.
	poolCacheSyncManager *poolScopedCacheSyncManager
	// poolCacheSyncedDector is used to detect if pool cache is synced and ready for use.
	// It will list/watch the informer sync lease, and if it's renewed by leader yurthub, isPoolCacheSynced will
	// be set as true which means the pool cache is ready for use. It also starts a routine which will set
	// isPoolCacheSynced as false if the informer sync lease has not been updated for a duration.
	poolCacheSyncedDetector *poolCacheSyncedDetector
	// delegateNodeLeaseManager is used to list/watch kube-node-lease from yurtcoordinator. If the
	// node lease contains DelegateHeartBeat label, it will triger the eventhandler which will
	// use cloud client to send it to cloud APIServer.
	delegateNodeLeaseManager *coordinatorLeaseInformerManager
}

func NewCoordinator(
	ctx context.Context,
	cfg *config.YurtHubConfiguration,
	cloudHealthChecker healthchecker.MultipleBackendsHealthChecker,
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
		return nil, fmt.Errorf("could not create client for yurt coordinator, %v", err)
	}

	coordinator := &coordinator{
		ctx:                ctx,
		cloudCAFileData:    cfg.CertManager.GetCAData(),
		cloudHealthChecker: cloudHealthChecker,
		etcdStorageCfg:     etcdStorageCfg,
		restConfigMgr:      restMgr,
		certMgr:            certMgr,
		informerFactory:    cfg.SharedFactory,
		diskStorage:        cfg.StorageWrapper.GetStorage(),
		serializerMgr:      cfg.SerializerManager,
		restMapperMgr:      cfg.RESTMapperManager,
		hubElector:         elector,
		statusInfoChan:     make(chan statusInfo, 10),
	}

	poolCacheSyncedDetector := &poolCacheSyncedDetector{
		ctx:            ctx,
		updateNotifyCh: make(chan struct{}),
		syncLeaseManager: &coordinatorLeaseInformerManager{
			ctx:               ctx,
			coordinatorClient: coordinatorClient,
		},
		staleTimeout: defaultPoolCacheStaleDuration,
		isPoolCacheSyncSetter: func(value bool) {
			coordinator.Lock()
			defer coordinator.Unlock()
			coordinator.isPoolCacheSynced = value
		},
	}

	delegateNodeLeaseManager := &coordinatorLeaseInformerManager{
		ctx:               ctx,
		coordinatorClient: coordinatorClient,
	}

	proxiedClient, err := buildProxiedClientWithUserAgent(fmt.Sprintf("http://%s", cfg.YurtHubProxyServerAddr), constants.DefaultPoolScopedUserAgent)
	if err != nil {
		return nil, fmt.Errorf("could not create proxied client, %v", err)
	}

	// init pool scope resources
	resources.InitPoolScopeResourcesManger(proxiedClient, cfg.SharedFactory)

	dynamicClient, err := buildDynamicClientWithUserAgent(fmt.Sprintf("http://%s", cfg.YurtHubProxyServerAddr), constants.DefaultPoolScopedUserAgent)
	if err != nil {
		return nil, fmt.Errorf("could not create dynamic client, %v", err)
	}

	poolScopedCacheSyncManager := &poolScopedCacheSyncManager{
		ctx:               ctx,
		dynamicClient:     dynamicClient,
		coordinatorClient: coordinatorClient,
		nodeName:          cfg.NodeName,
		getEtcdStore:      coordinator.getEtcdStore,
	}

	coordinator.poolCacheSyncedDetector = poolCacheSyncedDetector
	coordinator.delegateNodeLeaseManager = delegateNodeLeaseManager
	coordinator.poolCacheSyncManager = poolScopedCacheSyncManager

	return coordinator, nil
}

func (coordinator *coordinator) Run() {
	// waiting for pool scope resource synced
	resources.WaitUntilPoolScopeResourcesSync(coordinator.ctx)

	for {
		var poolCacheManager cachemanager.CacheManager
		var cancelEtcdStorage = func() {}
		var needUploadLocalCache bool
		var needCancelEtcdStorage bool
		var isPoolCacheSynced bool
		var etcdStorage storage.Store
		var err error

		select {
		case <-coordinator.ctx.Done():
			coordinator.poolCacheSyncManager.EnsureStop()
			coordinator.delegateNodeLeaseManager.EnsureStop()
			coordinator.poolCacheSyncedDetector.EnsureStop()
			klog.Info("exit normally in coordinator loop.")
			return
		case electorStatus, ok := <-coordinator.hubElector.StatusChan():
			if !ok {
				return
			}
			metrics.Metrics.ObserveYurtCoordinatorYurthubRole(electorStatus)

			if coordinator.cnt == math.MaxUint64 {
				// cnt will overflow, reset it.
				coordinator.cnt = 0
				// if statusInfoChan channel also has data, clean it.
				length := len(coordinator.statusInfoChan)
				if length > 0 {
					i := 0
					for v := range coordinator.statusInfoChan {
						klog.Infof("clean statusInfo data %+v when coordinator.cnt is reset", v)
						i++
						if i == length {
							break
						}
					}
				}
			}
			coordinator.cnt++
			coordinator.statusInfoChan <- statusInfo{
				electorStatus: electorStatus,
				currentCnt:    coordinator.cnt,
			}
		case electorStatusInfo, ok := <-coordinator.statusInfoChan:
			if !ok {
				return
			}
			if electorStatusInfo.currentCnt < coordinator.cnt {
				klog.Infof("electorStatusInfo %+v is behind of current cnt %d", electorStatusInfo, coordinator.cnt)
				continue
			}

			switch electorStatusInfo.electorStatus {
			case PendingHub:
				coordinator.poolCacheSyncManager.EnsureStop()
				coordinator.delegateNodeLeaseManager.EnsureStop()
				coordinator.poolCacheSyncedDetector.EnsureStop()
				needUploadLocalCache = true
				needCancelEtcdStorage = true
				isPoolCacheSynced = false
				etcdStorage = nil
				poolCacheManager = nil
			case LeaderHub:
				poolCacheManager, etcdStorage, cancelEtcdStorage, err = coordinator.buildPoolCacheStore()
				if err != nil {
					klog.Errorf("could not create pool scoped cache store and manager, %v", err)
					coordinator.statusInfoChan <- electorStatusInfo
					continue
				}

				if err := coordinator.poolCacheSyncManager.EnsureStart(); err != nil {
					klog.Errorf("could not sync pool-scoped resource, %v", err)
					cancelEtcdStorage()
					coordinator.statusInfoChan <- electorStatusInfo
					continue
				}
				klog.Infof("coordinator poolCacheSyncManager has ensure started")

				nodeLeaseProxyClient, err := coordinator.newNodeLeaseProxyClient()
				if err != nil {
					klog.Errorf("cloud not get cloud lease client when becoming leader yurthub, %v", err)
					cancelEtcdStorage()
					coordinator.statusInfoChan <- electorStatusInfo
					continue
				}
				klog.Infof("coordinator newCloudLeaseClient success.")
				coordinator.delegateNodeLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
					FilterFunc: ifDelegateHeartBeat,
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: func(obj interface{}) {
							coordinator.delegateNodeLease(nodeLeaseProxyClient, obj)
						},
						UpdateFunc: func(_, newObj interface{}) {
							coordinator.delegateNodeLease(nodeLeaseProxyClient, newObj)
						},
					},
				})

				coordinator.poolCacheSyncedDetector.EnsureStart()

				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(etcdStorage); err != nil {
						klog.Errorf("could not upload local cache when yurthub becomes leader, %v", err)
					} else {
						needUploadLocalCache = false
					}
				}
			case FollowerHub:
				poolCacheManager, etcdStorage, cancelEtcdStorage, err = coordinator.buildPoolCacheStore()
				if err != nil {
					klog.Errorf("could not create pool scoped cache store and manager, %v", err)
					coordinator.statusInfoChan <- electorStatusInfo
					continue
				}

				coordinator.poolCacheSyncManager.EnsureStop()
				coordinator.delegateNodeLeaseManager.EnsureStop()
				coordinator.poolCacheSyncedDetector.EnsureStart()

				if coordinator.needUploadLocalCache {
					if err := coordinator.uploadLocalCache(etcdStorage); err != nil {
						klog.Errorf("could not upload local cache when yurthub becomes follower, %v", err)
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
				cancelEtcdStorage()
			}
			coordinator.electStatus = electorStatusInfo.electorStatus
			coordinator.poolCacheManager = poolCacheManager
			coordinator.etcdStorage = etcdStorage
			coordinator.cancelEtcdStorage = cancelEtcdStorage
			coordinator.needUploadLocalCache = needUploadLocalCache
			coordinator.isPoolCacheSynced = isPoolCacheSynced
			coordinator.Unlock()
		}
	}
}

// IsReady will return the poolCacheManager and true if the yurt-coordinator is ready.
// Yurt-Coordinator ready means it is ready to handle request. To be specific, it should
// satisfy the following 3 condition:
// 1. Yurt-Coordinator is healthy
// 2. Pool-Scoped resources have been synced with cloud, through list/watch
// 3. local cache has been uploaded to yurt-coordinator
func (coordinator *coordinator) IsReady() (cachemanager.CacheManager, bool) {
	// If electStatus is not PendingHub, it means yurt-coordinator is healthy.
	coordinator.Lock()
	defer coordinator.Unlock()
	if coordinator.electStatus != PendingHub && coordinator.isPoolCacheSynced && !coordinator.needUploadLocalCache {
		metrics.Metrics.ObserveYurtCoordinatorReadyStatus(1)
		return coordinator.poolCacheManager, true
	}
	metrics.Metrics.ObserveYurtCoordinatorReadyStatus(0)
	return nil, false
}

// IsCoordinatorHealthy will return the poolCacheManager and true if the yurt-coordinator is healthy.
// We assume coordinator is healthy when the elect status is LeaderHub and FollowerHub.
func (coordinator *coordinator) IsHealthy() (cachemanager.CacheManager, bool) {
	coordinator.Lock()
	defer coordinator.Unlock()
	if coordinator.electStatus != PendingHub {
		metrics.Metrics.ObserveYurtCoordinatorHealthyStatus(1)
		return coordinator.poolCacheManager, true
	}
	metrics.Metrics.ObserveYurtCoordinatorHealthyStatus(0)
	return nil, false
}

func (coordinator *coordinator) buildPoolCacheStore() (cachemanager.CacheManager, storage.Store, func(), error) {
	ctx, cancel := context.WithCancel(coordinator.ctx)
	etcdStore, err := etcd.NewStorage(ctx, coordinator.etcdStorageCfg)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("could not create etcd storage, %v", err)
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

func (coordinator *coordinator) newNodeLeaseProxyClient() (coordclientset.LeaseInterface, error) {
	healthyCloudServer, err := coordinator.cloudHealthChecker.PickHealthyServer()
	if err != nil {
		return nil, fmt.Errorf("could not get a healthy cloud APIServer, %v", err)
	} else if healthyCloudServer == nil {
		return nil, fmt.Errorf("could not get a healthy cloud APIServer, all server are unhealthy")
	}
	restCfg := &rest.Config{
		Host: healthyCloudServer.String(),
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   coordinator.cloudCAFileData,
			CertFile: coordinator.certMgr.GetFilePath(certmanager.NodeLeaseProxyClientCert),
			KeyFile:  coordinator.certMgr.GetFilePath(certmanager.NodeLeaseProxyClientKey),
		},
		Timeout: 10 * time.Second,
	}
	cloudClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("could not create cloud client, %v", err)
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
		// ResourceVersions of lease objects in yurt-coordinator always have different rv
		// from what of cloud lease. So we should get cloud lease first and then update
		// it with lease from yurt-coordinator.
		cloudLease, err := cloudLeaseClient.Get(coordinator.ctx, newLease.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := cloudLeaseClient.Create(coordinator.ctx, cloudLease, metav1.CreateOptions{}); err != nil {
				klog.Errorf("could not create lease %s at cloud, %v", newLease.Name, err)
				continue
			}
		}

		cloudLease.Annotations = newLease.Annotations
		cloudLease.Spec.RenewTime = newLease.Spec.RenewTime
		if updatedLease, err := cloudLeaseClient.Update(coordinator.ctx, cloudLease, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("could not update lease %s at cloud, %v", newLease.Name, err)
			continue
		} else {
			klog.V(2).Infof("delegate node lease for %s", updatedLease.Name)
		}
		break
	}
}

// poolScopedCacheSyncManager will continuously sync pool-scoped resources from cloud to yurt-coordinator.
// After resource sync is completed, it will periodically renew the informer synced lease, which is used by
// other yurthub to determine if yurt-coordinator is ready to handle requests of pool-scoped resources.
// It uses proxied client to list/watch pool-scoped resources from cloud APIServer, which
// will be automatically cached into yurt-coordinator through YurtProxyServer.
type poolScopedCacheSyncManager struct {
	ctx       context.Context
	isRunning bool
	// dynamicClient is a dynamic client of Cloud APIServer which is proxied by yurthub.
	dynamicClient dynamic.Interface
	// coordinatorClient is a client of APIServer in yurt-coordinator.
	coordinatorClient kubernetes.Interface
	// nodeName will be used to update the ownerReference of informer synced lease.
	nodeName            string
	informerSyncedLease *coordinationv1.Lease
	getEtcdStore        func() storage.Store
	cancel              func()
}

func (p *poolScopedCacheSyncManager) EnsureStart() error {
	if !p.isRunning {
		err := p.coordinatorClient.CoordinationV1().Leases(namespaceInformerLease).Delete(p.ctx, nameInformerLease, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("could not delete informer sync lease, %v", err)
		}

		etcdStore := p.getEtcdStore()
		if etcdStore == nil {
			return fmt.Errorf("got empty etcd storage")
		}
		if err := etcdStore.DeleteComponentResources(constants.DefaultPoolScopedUserAgent); err != nil {
			return fmt.Errorf("could not clean old pool-scoped cache, %v", err)
		}

		ctx, cancel := context.WithCancel(p.ctx)
		hasInformersSynced := []cache.InformerSynced{}
		dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(p.dynamicClient, 0, metav1.NamespaceAll, nil)
		for _, gvr := range resources.GetPoolScopeResources() {
			klog.Infof("coordinator informer with resources gvr %+v registered", gvr)
			informer := dynamicInformerFactory.ForResource(gvr)
			hasInformersSynced = append(hasInformersSynced, informer.Informer().HasSynced)
		}

		dynamicInformerFactory.Start(ctx.Done())
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
	klog.Error("could not wait for cache synced, it was canceled")
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
				klog.Errorf("could not update informer lease, %v", err)
				continue
			}
			p.informerSyncedLease = newLease
		}
	}
}

// coordinatorLeaseInformerManager will use yurt-coordinator client to list/watch
// lease in yurt-coordinator. Through passing different event handler, it can either
// delegating node lease by leader yurthub or detecting the informer synced lease to
// check if yurt-coordinator is ready for requests of pool-scoped resources.
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
// Currently, we only upload pods and nodes to yurt-coordinator.
type localCacheUploader struct {
	diskStorage storage.Store
	etcdStorage storage.Store
}

func (l *localCacheUploader) Upload() {
	objBytes := l.resourcesToUpload()
	for k, b := range objBytes {
		rv, err := getRv(b)
		if err != nil {
			klog.Errorf("could not get name from bytes %s, %v", string(b), err)
			continue
		}

		if err := l.createOrUpdate(k, b, rv); err != nil {
			klog.Errorf("could not upload %s, %v", k.Key(), err)
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
		localKeys, err := l.diskStorage.ListResourceKeysOfComponent(info.Component, gvr)
		if err != nil {
			klog.Errorf("could not get object keys from disk for %s, %v", gvr.String(), err)
			continue
		}

		for _, k := range localKeys {
			buf, err := l.diskStorage.Get(k)
			if err != nil {
				klog.Errorf("could not read local cache of key %s, %v", k.Key(), err)
				continue
			}
			buildInfo, err := disk.ExtractKeyBuildInfo(k)
			if err != nil {
				klog.Errorf("could not extract key build info from local cache of key %s, %v", k.Key(), err)
				continue
			}

			poolCacheKey, err := l.etcdStorage.KeyFunc(*buildInfo)
			if err != nil {
				klog.Errorf("could not generate pool cache key from local cache key %s, %v", k.Key(), err)
				continue
			}
			objBytes[poolCacheKey] = buf
		}
	}
	return objBytes
}

// poolCacheSyncedDector will list/watch informer-sync-lease to detect if pool cache can be used.
// The leader yurthub should periodically renew the lease. If the lease is not updated for staleTimeout
// duration, it will think the pool cache cannot be used.
type poolCacheSyncedDetector struct {
	ctx            context.Context
	updateNotifyCh chan struct{}
	isRunning      bool
	staleTimeout   time.Duration
	// syncLeaseManager is used to list/watch the informer-sync-lease, and set the
	// isPoolCacheSync as ture when it is renewed.
	syncLeaseManager      *coordinatorLeaseInformerManager
	isPoolCacheSyncSetter func(value bool)
	cancelLoop            func()
}

func (p *poolCacheSyncedDetector) EnsureStart() {
	if !p.isRunning {
		p.syncLeaseManager.EnsureStartWithHandler(cache.FilteringResourceEventHandler{
			FilterFunc: ifInformerSyncLease,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: p.detectPoolCacheSynced,
				UpdateFunc: func(_, newObj interface{}) {
					p.detectPoolCacheSynced(newObj)
				},
				DeleteFunc: func(_ interface{}) {
					p.isPoolCacheSyncSetter(false)
				},
			},
		})

		ctx, cancel := context.WithCancel(p.ctx)
		p.cancelLoop = cancel
		p.isRunning = true
		go p.loopForChange(ctx)
	}
}

func (p *poolCacheSyncedDetector) EnsureStop() {
	if p.isRunning {
		p.syncLeaseManager.EnsureStop()
		p.cancelLoop()
		p.isRunning = false
	}
}

func (p *poolCacheSyncedDetector) loopForChange(ctx context.Context) {
	t := time.NewTicker(p.staleTimeout)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.updateNotifyCh:
			t.Reset(p.staleTimeout)
			p.isPoolCacheSyncSetter(true)
		case <-t.C:
			klog.V(4).Infof("timeout waitting for pool cache sync lease being updated, do not use pool cache")
			p.isPoolCacheSyncSetter(false)
		}
	}
}

func (p *poolCacheSyncedDetector) detectPoolCacheSynced(obj interface{}) {
	lease := obj.(*coordinationv1.Lease)
	renewTime := lease.Spec.RenewTime
	if time.Now().Before(renewTime.Add(p.staleTimeout)) {
		// The lease is updated before pool cache being considered as stale.
		p.updateNotifyCh <- struct{}{}
	}
}

func getRv(objBytes []byte) (uint64, error) {
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(objBytes, obj); err != nil {
		return 0, fmt.Errorf("could not unmarshal json: %v", err)
	}

	rv, err := strconv.ParseUint(obj.GetResourceVersion(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse rv %s of pod %s, %v", obj.GetName(), obj.GetResourceVersion(), err)
	}

	return rv, nil
}

func ifDelegateHeartBeat(obj interface{}) bool {
	lease, ok := obj.(*coordinationv1.Lease)
	if !ok {
		return false
	}
	v, ok := lease.Annotations[healthchecker.DelegateHeartBeat]
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

func buildDynamicClientWithUserAgent(proxyAddr string, userAgent string) (dynamic.Interface, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(proxyAddr, "")
	if err != nil {
		return nil, err
	}

	kubeConfig.UserAgent = userAgent
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}
