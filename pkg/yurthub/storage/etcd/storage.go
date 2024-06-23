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
	"crypto/tls"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	clientv3 "go.etcd.io/etcd/client/v3"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/utils"
)

const (
	StorageName                   = "yurt-coordinator"
	defaultTimeout                = 5 * time.Second
	defaultHealthCheckPeriod      = 10 * time.Second
	defaultDialTimeout            = 10 * time.Second
	defaultMaxSendSize            = 100 * 1024 * 1024
	defaultMaxReceiveSize         = 100 * 1024 * 1024
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
	UnSecure      bool
}

// TODO: consider how to recover the work if it was interrupted because of restart, in
// which case we've added/deleted key in local cache but failed to add/delete it in etcd.
type etcdStorage struct {
	ctx             context.Context
	prefix          string
	mirrorPrefixMap map[pathType]string
	client          *clientv3.Client
	clientConfig    clientv3.Config
	serializer      runtime.Serializer
	// For etcd storage, we do not need to cache cluster info, because
	// we can get it form apiserver in yurt-coordinator.
	doNothingAboutClusterInfo
}

func NewStorage(ctx context.Context, cfg *EtcdStorageConfig) (storage.Store, error) {
	var tlsConfig *tls.Config
	var err error
	if !cfg.UnSecure {
		tlsInfo := transport.TLSInfo{
			CertFile:      cfg.CertFile,
			KeyFile:       cfg.KeyFile,
			TrustedCAFile: cfg.CaFile,
		}

		tlsConfig, err = tlsInfo.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("could not create tls config for etcd client, %v", err)
		}
	}

	clientConfig := clientv3.Config{
		Endpoints:          cfg.EtcdEndpoints,
		TLS:                tlsConfig,
		DialTimeout:        defaultDialTimeout,
		MaxCallRecvMsgSize: defaultMaxReceiveSize,
		MaxCallSendMsgSize: defaultMaxSendSize,
	}

	client, err := clientv3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create etcd client, %v", err)
	}

	s := &etcdStorage{
		ctx:          ctx,
		prefix:       cfg.Prefix,
		client:       client,
		clientConfig: clientConfig,
		serializer:   json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{}),
		mirrorPrefixMap: map[pathType]string{
			rvType: "/mirror/rv",
		},
	}

	go s.clientLifeCycleManagement()

	return s, nil
}

func (s *etcdStorage) mirrorPath(path string, pathType pathType) string {
	return filepath.Join(s.mirrorPrefixMap[pathType], path)
}

func (s *etcdStorage) Name() string {
	return StorageName
}

func (s *etcdStorage) clientLifeCycleManagement() {
	reconnect := func(ctx context.Context) {
		t := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if client, err := clientv3.New(s.clientConfig); err == nil {
					klog.Infof("client reconnected to etcd server, %s", client.ActiveConnection().GetState().String())
					if err := s.client.Close(); err != nil {
						klog.Errorf("could not close old client, %v", err)
					}
					s.client = client
					return
				}
				continue
			}
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			if err := s.client.Close(); err != nil {
				klog.Errorf("could not close etcd client, %v", err)
			}
			klog.Info("etcdstorage lifecycle routine exited")
			return
		default:
			timeoutCtx, cancel := context.WithTimeout(s.ctx, defaultDialTimeout)
			healthCli := healthpb.NewHealthClient(s.client.ActiveConnection())
			resp, err := healthCli.Check(timeoutCtx, &healthpb.HealthCheckRequest{})
			// We should call cancel in case Check request does not timeout, to release resource.
			cancel()
			if err != nil {
				klog.Errorf("check health of etcd failed, err: %v, try to reconnect", err)
				reconnect(s.ctx)
			} else if resp != nil && resp.Status != healthpb.HealthCheckResponse_SERVING {
				klog.Errorf("unexpected health status from etcd, status: %s", resp.Status.String())
			}
			time.Sleep(defaultHealthCheckPeriod)
		}
	}
}

func (s *etcdStorage) Create(key storage.Key, obj runtime.Object) error {
	if err := utils.ValidateKV(key, obj, storageKey{}); err != nil {
		return err
	}
	content, err := runtime.Encode(s.serializer, obj)
	if err != nil {
		return err
	}

	keyStr := key.Key()
	originRv, err := getRvOfObject(content)
	if err != nil {
		return fmt.Errorf("could not get rv from content when creating %s, %v", keyStr, err)
	}

	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()
	txnResp, err := s.client.KV.Txn(ctx).If(
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

	return nil
}

func (s *etcdStorage) Delete(key storage.Key) error {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return err
	}

	keyStr := key.Key()
	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()
	_, err := s.client.Txn(ctx).If().Then(
		clientv3.OpDelete(keyStr, clientv3.WithPrefix()),
		clientv3.OpDelete(s.mirrorPath(keyStr, rvType), clientv3.WithPrefix()),
	).Commit()
	if err != nil {
		return err
	}

	return nil
}

func (s *etcdStorage) Get(key storage.Key) (runtime.Object, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return nil, err
	}

	keyStr := key.Key()
	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()
	getResp, err := s.client.Get(ctx, keyStr)
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return nil, storage.ErrStorageNotFound
	}

	obj, err := runtime.Decode(s.serializer, getResp.Kvs[0].Value)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// TODO: When using etcd, do we have the case:
//	"If the rootKey exists in the store but no keys has the prefix of rootKey"?

func (s *etcdStorage) List(key storage.Key) ([]runtime.Object, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return nil, err
	}

	rootKeyStr := key.Key()
	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()
	getResp, err := s.client.Get(ctx, rootKeyStr, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return nil, storage.ErrStorageNotFound
	}
	objs := make([]runtime.Object, 0, len(getResp.Kvs))
	for _, kv := range getResp.Kvs {
		obj, err := runtime.Decode(s.serializer, kv.Value)
		if err != nil {
			klog.Errorf("failed to decode object %s: %w", string(kv.Key), err)
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func (s *etcdStorage) Update(key storage.Key, obj runtime.Object, rv uint64) (runtime.Object, error) {
	if err := utils.ValidateKV(key, obj, storageKey{}); err != nil {
		return nil, err
	}
	content, err := runtime.Encode(s.serializer, obj)
	if err != nil {
		return nil, err
	}

	keyStr := key.Key()
	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()

	getResp, err := s.client.Get(context.TODO(), s.mirrorPath(keyStr, rvType))
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		_, err := s.client.Txn(ctx).If().Then(
			clientv3.OpPut(keyStr, string(content)),
			clientv3.OpPut(s.mirrorPath(keyStr, rvType), fixLenRvUint64(rv)),
		).Commit()
		if err != nil {
			return nil, err
		}
		return obj, nil
	}

	oldRv := string(getResp.Kvs[0].Value)
	txnResp, err := s.client.KV.Txn(ctx).If(
		fresherThan(fixLenRvUint64(rv), oldRv),
	).Then(
		clientv3.OpPut(keyStr, string(content)),
		clientv3.OpPut(s.mirrorPath(keyStr, rvType), fixLenRvUint64(rv)),
	).Commit()
	if err != nil {
		return nil, err
	}

	if !txnResp.Succeeded {
		oldObj, err := runtime.Decode(s.serializer, getResp.Kvs[0].Value)
		if err != nil {
			return obj, nil
		}
		return oldObj, storage.ErrUpdateConflict
	}
	return obj, nil
}

func (s *etcdStorage) ListKeys(key storage.Key) ([]storage.Key, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	getResp, err := s.client.Get(ctx, key.Key(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return nil, storage.ErrStorageNotFound
	}
	keys := make([]storage.Key, 0, len(getResp.Kvs))
	for k := range getResp.Kvs {
		keys = append(keys, storageKey{
			path: string(k),
		})
	}
	return keys, nil
}

func (s *etcdStorage) Replace(key storage.Key, objs map[storage.Key]runtime.Object) error {
	rootKey := key.(storageKey)
	contents := make(map[storage.Key][]byte)
	for k, obj := range objs {
		_, ok := k.(storageKey)
		if !ok {
			return storage.ErrUnrecognizedKey
		}
		if !strings.HasPrefix(k.Key(), rootKey.Key()) {
			return storage.ErrInvalidContent
		}
		content, err := runtime.Encode(s.serializer, obj)
		if err != nil {
			return err
		}
		contents[k] = content
	}

	ops := []clientv3.Op{}
	ops = append(ops, clientv3.OpDelete(key.Key(), clientv3.WithPrefix()))
	for k, content := range contents {
		rv, err := getRvOfObject(content)
		if err != nil {
			return err
		}
		ops = append(ops,
			clientv3.OpPut(k.Key(), string(content)),
			clientv3.OpPut(s.mirrorPath(k.Key(), rvType), fixLenRvString(rv)))
	}

	ctx, cancel := context.WithTimeout(s.ctx, defaultTimeout)
	defer cancel()
	_, err := s.client.Txn(ctx).If().Then(ops...).Commit()
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
