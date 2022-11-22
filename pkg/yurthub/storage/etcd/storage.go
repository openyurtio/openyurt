package etcd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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
	StorageName        = "pool-coordinator"
	defaultDialTimeout = 10 * time.Second
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
}

type keySet map[storage.Key]struct{}

// Difference will return keys in s but not in s2
func (s keySet) Difference(s2 keySet) []storage.Key {
	keys := []storage.Key{}
	for k := range s {
		if _, ok := s2[k]; !ok {
			keys = append(keys, k)
		}
	}
	return keys
}

type etcdStorage struct {
	ctx             context.Context
	prefix          string
	mirrorPrefixMap map[pathType]string
	client          *clientv3.Client
	// map component name to its key cache
	localComponentKeyCache sync.Map
	// For etcd storage, we do not need to cache cluster info, because
	// we can get it form apiserver in pool-coordinator.
	doNothingAboutClusterInfo
}

func NewStorage(ctx context.Context, cfg *EtcdStorageConfig) (storage.Store, error) {
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
		ctx:    ctx,
		prefix: cfg.Prefix,
		client: client,
		mirrorPrefixMap: map[pathType]string{
			rvType: "/mirror/rv",
		},
	}, nil
}

func (s *etcdStorage) mirrorPath(path string, pathType pathType) string {
	return filepath.Join(s.mirrorPrefixMap[pathType])
}

func (s *etcdStorage) Name() string {
	return StorageName
}

func (s *etcdStorage) Create(key storage.Key, content []byte) error {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return err
	}

	keyStr := key.Key()
	originRv, err := getRvOfObject(content)
	if err != nil {
		return fmt.Errorf("failed to get rv from content when creating %s", keyStr)
	}
	txnResp, err := s.client.KV.Txn(s.ctx).If(
		notFound(keyStr),
	).Then(
		clientv3.OpPut(keyStr, string(content)),
		clientv3.OpPut(s.mirrorPath(keyStr, rvType), originRv),
	).Commit()

	if err != nil {
		return err
	}

	if !txnResp.Succeeded {
		return storage.ErrKeyExists
	}
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

	if !key.IsRootKey() {
		return nil, storage.ErrIsNotRootKey
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
		clientv3.Compare(clientv3.Value(s.mirrorPath(keyStr, rvType)), "<", fmt.Sprintf("%d", rv)),
	).Then(
		clientv3.OpPut(keyStr, string(content)),
		clientv3.OpPut(s.mirrorPath(keyStr, rvType), fmt.Sprintf("%d", rv)),
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
		Resources: gvr.Resource,
		Group:     gvr.Group,
		Version:   gvr.Version,
	})
	if err != nil {
		return nil, err
	}

	keys := []storage.Key{}
	v, ok := s.localComponentKeyCache.Load(component)
	if !ok {
		return nil, storage.ErrStorageNotFound
	}
	keyCache := v.(keySet)
	for k := range keyCache {
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

	oldKeyCache, newKeyCache := keySet{}, keySet{}
	for k := range contents {
		newKeyCache[k] = struct{}{}
	}
	value, loaded := s.localComponentKeyCache.LoadOrStore(component, newKeyCache)
	if loaded {
		oldKeyCache = value.(keySet)
	}
	added := newKeyCache.Difference(oldKeyCache)
	deleted := oldKeyCache.Difference(newKeyCache)

	ops := []clientv3.Op{}
	for _, k := range added {
		putOp := clientv3.OpPut(k.Key(), string(contents[k]))
		ops = append(ops, putOp)
	}
	for _, k := range deleted {
		delOp := clientv3.OpDelete(k.Key())
		ops = append(ops, delOp)
	}

	// TODO: consider how to recover the work if it was interrupted because of restart, in
	// which case we've deleted key in local cache but failed to delete it from etcd.
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
	value, loaded := s.localComponentKeyCache.LoadAndDelete(component)
	if !loaded {
		// no need to delete
		return nil
	}

	keyCache := value.(keySet)
	ops := []clientv3.Op{}
	for k := range keyCache {
		delOp := clientv3.OpDelete(k.Key())
		ops = append(ops, delOp)
	}

	// TODO: consider how to recover the work if it was interrupted because of restart, in
	// which case we've deleted key in local cache but failed to delete it from etcd.
	_, err := s.client.Txn(s.ctx).If().Then(ops...).Commit()
	if err != nil {
		return err
	}
	return nil
}

// TODO: do not get rv through decoding, which means we have to
// unmarshal bytes. We should not do any serialization in storage.
func getRvOfObject(object []byte) (string, error) {
	decoder := scheme.Codecs.UniversalDeserializer()
	unstructuredObj := new(unstructured.Unstructured)
	_, _, err := decoder.Decode(object, nil, unstructuredObj)
	if err != nil {
		return "", fmt.Errorf("failed to get rv, %v", err)
	}

	return unstructuredObj.GetResourceVersion(), nil
}

func notFound(key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), "=", 0)
}

func found(key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), ">", 0)
}

type doNothingAboutClusterInfo struct{}

func (d doNothingAboutClusterInfo) SaveClusterInfo(_ storage.ClusterInfoKey, _ []byte) error {
	return nil
}
func (d doNothingAboutClusterInfo) GetClusterInfo(_ storage.ClusterInfoKey) ([]byte, error) {
	return nil, nil
}
