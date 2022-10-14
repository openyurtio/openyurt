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

package etcd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/utils"
)

const (
	StorageName                   = "pool-coordinator"
	defaultDialTimeout            = 10 * time.Second
	defaultComponentCacheFileName = "component-key-cache"
	defaultRvLen                  = 32
)

type pathType string

var (
	rvType pathType = "rv"
)

type EtcdStorageConfig struct {
	Prefix        string
	EtcdEndpoints []string
	CertFile      string
	KeyFile       string
	CaFile        string
	LocalCacheDir string
}

// TODO: consider how to recover the work if it was interrupted because of restart, in
// which case we've added/deleted key in local cache but failed to add/delete it in etcd.
type etcdStorage struct {
	ctx             context.Context
	prefix          string
	mirrorPrefixMap map[pathType]string
	client          *clientv3.Client
	// localComponentKeyCache persistently records keys owned by different components
	// It's useful to recover previous state when yurthub restarts.
	// We need this cache at local host instead of in etcd, because we need to ensure each
	// operation on etcd is atomic. If we store it in etcd, we have to get it first and then
	// do the action, such as ReplaceComponentList, which makes it non-atomic.
	// We assume that for resources listed by components on this node consist of two kinds:
	// 1. common resources: which are also used by other nodes
	// 2. special resources: which are only used by this nodes
	// In local cache, we do not need to bother to distinguish these two kinds.
	// For special resources, this node absolutely can create/update/delete them.
	// For common resources, thanks to list/watch we can ensure that resources in pool-coordinator
	// are finally consistent with the cloud, though there maybe a little jitter.
	localComponentKeyCache *componentKeyCache
	// For etcd storage, we do not need to cache cluster info, because
	// we can get it form apiserver in pool-coordinator.
	doNothingAboutClusterInfo
}

func NewStorage(ctx context.Context, cfg *EtcdStorageConfig) (storage.Store, error) {
	cacheFilePath := filepath.Join(cfg.LocalCacheDir, defaultComponentCacheFileName)
	cache := newComponentKeyCache(cacheFilePath)
	if err := cache.Recover(); err != nil {
		return nil, fmt.Errorf("failed to recover component key cache from %s, %v", cacheFilePath, err)
	}

	tlsInfo := transport.TLSInfo{
		CertFile:      cfg.CertFile,
		KeyFile:       cfg.KeyFile,
		TrustedCAFile: cfg.CaFile,
	}

	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create tls config for etcd client, %v", err)
	}

	clientConfig := clientv3.Config{
		Endpoints:   cfg.EtcdEndpoints,
		TLS:         tlsConfig,
		DialTimeout: defaultDialTimeout,
	}

	client, err := clientv3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client, %v", err)
	}

	go func() {
		// shutdown
		<-ctx.Done()
		if err := client.Close(); err != nil {
			klog.Errorf("failed to close the connection to etcd, %v", err)
		}
	}()

	return &etcdStorage{
		ctx:                    ctx,
		prefix:                 cfg.Prefix,
		client:                 client,
		localComponentKeyCache: cache,
		mirrorPrefixMap: map[pathType]string{
			rvType: "/mirror/rv",
		},
	}, nil
}

func (s *etcdStorage) mirrorPath(path string, pathType pathType) string {
	return filepath.Join(s.mirrorPrefixMap[pathType], path)
}

func (s *etcdStorage) Name() string {
	return StorageName
}

func (s *etcdStorage) Create(key storage.Key, content []byte) error {
	if err := utils.ValidateKV(key, content, storageKey{}); err != nil {
		return err
	}

	keyStr := key.Key()
	originRv, err := getRvOfObject(content)
	if err != nil {
		return fmt.Errorf("failed to get rv from content when creating %s, %v", keyStr, err)
	}
	txnResp, err := s.client.KV.Txn(s.ctx).If(
		notFound(keyStr),
	).Then(
		clientv3.OpPut(keyStr, string(content)),
		clientv3.OpPut(s.mirrorPath(keyStr, rvType), fixLenRvString(originRv)),
	).Commit()

	if err != nil {
		return err
	}

	if !txnResp.Succeeded {
		return storage.ErrKeyExists
	}

	storageKey := key.(storageKey)
	s.localComponentKeyCache.AddKey(storageKey.component(), storageKey)
	return nil
}

func (s *etcdStorage) Delete(key storage.Key) error {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return err
	}

	keyStr := key.Key()
	_, err := s.client.Txn(s.ctx).If().Then(
		clientv3.OpDelete(keyStr),
		clientv3.OpDelete(s.mirrorPath(keyStr, rvType)),
	).Commit()
	if err != nil {
		return err
	}

	storageKey := key.(storageKey)
	s.localComponentKeyCache.DeleteKey(storageKey.component(), storageKey)
	return nil
}

func (s *etcdStorage) Get(key storage.Key) ([]byte, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return nil, err
	}

	keyStr := key.Key()
	getResp, err := s.client.Get(s.ctx, keyStr)
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return nil, storage.ErrStorageNotFound
	}

	return getResp.Kvs[0].Value, nil
}

// TODO: When using etcd, do we have the case:
//	"If the rootKey exists in the store but no keys has the prefix of rootKey"?
func (s *etcdStorage) List(key storage.Key) ([][]byte, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return [][]byte{}, err
	}

	rootKeyStr := key.Key()
	getResp, err := s.client.Get(s.ctx, rootKeyStr, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return nil, storage.ErrStorageNotFound
	}

	values := make([][]byte, 0, len(getResp.Kvs))
	for _, kv := range getResp.Kvs {
		values = append(values, kv.Value)
	}
	return values, nil
}

func (s *etcdStorage) Update(key storage.Key, content []byte, rv uint64) ([]byte, error) {
	if err := utils.ValidateKV(key, content, storageKey{}); err != nil {
		return nil, err
	}

	keyStr := key.Key()
	txnResp, err := s.client.KV.Txn(s.ctx).If(
		found(keyStr),
		fresherThan(fixLenRvUint64(rv), s.mirrorPath(keyStr, rvType)),
	).Then(
		clientv3.OpPut(keyStr, string(content)),
		clientv3.OpPut(s.mirrorPath(keyStr, rvType), fixLenRvUint64(rv)),
	).Else(
		// Possibly we have two cases here:
		// 1. key does not exist
		// 2. key exists with a higher rv
		// We can distinguish them by OpGet. If it gets no value back, it's case 1.
		// Otherwise is case 2.
		clientv3.OpGet(keyStr),
	).Commit()

	if err != nil {
		return nil, err
	}

	if !txnResp.Succeeded {
		getResp := (*clientv3.GetResponse)(txnResp.Responses[0].GetResponseRange())
		if len(getResp.Kvs) == 0 {
			return nil, storage.ErrStorageNotFound
		}
		return getResp.Kvs[0].Value, storage.ErrUpdateConflict
	}

	return content, nil
}

func (s *etcdStorage) ListResourceKeysOfComponent(component string, gvr schema.GroupVersionResource) ([]storage.Key, error) {
	if component == "" {
		return nil, storage.ErrEmptyComponent
	}

	rootKey, err := s.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Resources: gvr.Resource,
		Group:     gvr.Group,
		Version:   gvr.Version,
	})
	if err != nil {
		return nil, err
	}

	keys := []storage.Key{}
	keyCache, ok := s.localComponentKeyCache.Load(component)
	if !ok {
		return nil, storage.ErrStorageNotFound
	}
	for k := range keyCache.m {
		if strings.HasPrefix(k.Key(), rootKey.Key()) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *etcdStorage) ReplaceComponentList(component string, gvr schema.GroupVersionResource, namespace string, contents map[storage.Key][]byte) error {
	if component == "" {
		return storage.ErrEmptyComponent
	}
	rootKey, err := s.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Resources: gvr.Resource,
		Group:     gvr.Group,
		Version:   gvr.Version,
		Namespace: namespace,
	})
	if err != nil {
		return err
	}
	for key := range contents {
		if !strings.HasPrefix(key.Key(), rootKey.Key()) {
			return storage.ErrInvalidContent
		}
	}

	newKeyCache := keySet{m: map[storageKey]struct{}{}}
	for k := range contents {
		storageKey, ok := k.(storageKey)
		if !ok {
			return storage.ErrUnrecognizedKey
		}
		newKeyCache.m[storageKey] = struct{}{}
	}
	var addedOrUpdated, deleted []storageKey
	oldKeyCache, loaded := s.localComponentKeyCache.LoadOrStore(component, newKeyCache)
	addedOrUpdated = newKeyCache.Difference(keySet{})
	if loaded {
		deleted = oldKeyCache.Difference(newKeyCache)
	}

	ops := []clientv3.Op{}
	for _, k := range addedOrUpdated {
		rv, err := getRvOfObject(contents[k])
		if err != nil {
			klog.Errorf("failed to process %s in list object, %v", k.Key(), err)
			continue
		}
		createOrUpdateOp := clientv3.OpTxn(
			[]clientv3.Cmp{
				// if
				found(k.Key()),
			},
			[]clientv3.Op{
				// then
				clientv3.OpTxn([]clientv3.Cmp{
					// if
					fresherThan(fixLenRvString(rv), s.mirrorPath(k.Key(), rvType)),
				}, []clientv3.Op{
					// then
					clientv3.OpPut(k.Key(), string(contents[k])),
					clientv3.OpPut(s.mirrorPath(k.Key(), rvType), fixLenRvString(rv)),
				}, []clientv3.Op{
					// else
					// do nothing
				}),
			},
			[]clientv3.Op{
				// else
				clientv3.OpPut(k.Key(), string(contents[k])),
				clientv3.OpPut(s.mirrorPath(k.Key(), rvType), fixLenRvString(rv)),
			},
		)
		ops = append(ops, createOrUpdateOp)
	}
	for _, k := range deleted {
		ops = append(ops,
			clientv3.OpDelete(k.Key()),
			clientv3.OpDelete(s.mirrorPath(k.Key(), rvType)),
		)
	}

	_, err = s.client.Txn(s.ctx).If().Then(ops...).Commit()
	if err != nil {
		return err
	}

	return nil
}

func (s *etcdStorage) DeleteComponentResources(component string) error {
	if component == "" {
		return storage.ErrEmptyComponent
	}
	keyCache, loaded := s.localComponentKeyCache.LoadAndDelete(component)
	if !loaded {
		// no need to delete
		return nil
	}

	ops := []clientv3.Op{}
	for k := range keyCache.m {
		ops = append(ops,
			clientv3.OpDelete(k.Key()),
			clientv3.OpDelete(s.mirrorPath(k.Key(), rvType)),
		)
	}

	_, err := s.client.Txn(s.ctx).If().Then(ops...).Commit()
	if err != nil {
		return err
	}
	return nil
}

func fixLenRvUint64(rv uint64) string {
	return fmt.Sprintf("%0*d", defaultRvLen, rv)
}

func fixLenRvString(rv string) string {
	return fmt.Sprintf("%0*s", defaultRvLen, rv)
}

// TODO: do not get rv through decoding, which means we have to
// unmarshal bytes. We should not do any serialization in storage.
func getRvOfObject(object []byte) (string, error) {
	decoder := scheme.Codecs.UniversalDeserializer()
	unstructuredObj := new(unstructured.Unstructured)
	_, _, err := decoder.Decode(object, nil, unstructuredObj)
	if err != nil {
		return "", err
	}

	return unstructuredObj.GetResourceVersion(), nil
}

func notFound(key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), "=", 0)
}

func found(key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), ">", 0)
}

func fresherThan(rv string, key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.Value(key), "<", rv)
}

type doNothingAboutClusterInfo struct{}

func (d doNothingAboutClusterInfo) SaveClusterInfo(_ storage.ClusterInfoKey, _ []byte) error {
	return nil
}
func (d doNothingAboutClusterInfo) GetClusterInfo(_ storage.ClusterInfoKey) ([]byte, error) {
	return nil, nil
}
