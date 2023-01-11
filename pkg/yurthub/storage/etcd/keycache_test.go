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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	coordinatorconstants "github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/constants"
	etcdmock "github.com/openyurtio/openyurt/pkg/yurthub/storage/etcd/mock"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

var _ = Describe("Test componentKeyCache setup", func() {
	var cache *componentKeyCache
	var fileName string
	var f fs.FileSystemOperator
	var mockedClient *clientv3.Client
	BeforeEach(func() {
		kv := etcdmock.KV{}
		kv.On("Get", "/registry/services/endpoints", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
			Return(&clientv3.GetResponse{})
		kv.On("Get", "/registry/endpointslices", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
			Return(&clientv3.GetResponse{})
		etcdStorage := &etcdStorage{prefix: "/registry"}
		mockedClient = &clientv3.Client{KV: kv}
		fileName = uuid.New().String()
		cache = &componentKeyCache{
			ctx:        context.Background(),
			filePath:   filepath.Join(keyCacheDir, fileName),
			cache:      map[string]keySet{},
			fsOperator: fs.FileSystemOperator{},
			etcdClient: mockedClient,
			keyFunc:    etcdStorage.KeyFunc,
		}
	})
	AfterEach(func() {
		Expect(os.RemoveAll(filepath.Join(keyCacheDir, fileName)))
	})

	It("should recover when cache file does not exist", func() {
		Expect(cache.Recover()).To(BeNil())
		Expect(len(cache.cache)).To(Equal(1))
	})

	It("should recover when cache file is empty", func() {
		Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte{})).To(BeNil())
		Expect(cache.Recover()).To(BeNil())
		Expect(len(cache.cache)).To(Equal(1))
	})

	Context("Test get pool-scoped resource keys from etcd", func() {
		BeforeEach(func() {
			kv := etcdmock.KV{}
			kv.On("Get", "/registry/services/endpoints", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
				Return(&clientv3.GetResponse{
					Kvs: []*mvccpb.KeyValue{
						{Key: []byte("/registry/services/endpoints/default/nginx")},
						{Key: []byte("/registry/services/endpoints/kube-system/kube-dns")},
					},
				})
			kv.On("Get", "/registry/endpointslices", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
				Return(&clientv3.GetResponse{
					Kvs: []*mvccpb.KeyValue{
						{Key: []byte("/registry/endpointslices/default/nginx")},
						{Key: []byte("/registry/endpointslices/kube-system/kube-dns")},
					},
				})
			mockedClient.KV = kv
		})

		It("should recover leader-yurthub cache from etcd", func() {
			Expect(cache.Recover()).To(BeNil())
			Expect(cache.cache[coordinatorconstants.DefaultPoolScopedUserAgent]).Should(Equal(
				keySet{
					m: map[storageKey]struct{}{
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/services/endpoints/default/nginx",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/services/endpoints/kube-system/kube-dns",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/endpointslices/default/nginx",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/endpointslices/kube-system/kube-dns",
						}: {},
					},
				},
			))
		})

		It("should replace leader-yurthub cache read from local file with keys from etcd", func() {
			Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte(
				"leader-yurthub:/registry/services/endpoints/default/nginx-local,"+
					"/registry/services/endpoints/kube-system/kube-dns-local,"+
					"/registry/endpointslices/default/nginx-local,"+
					"/registry/endpointslices/kube-system/kube-dns-local",
			))).To(BeNil())
			Expect(cache.Recover()).To(BeNil())
			Expect(cache.cache[coordinatorconstants.DefaultPoolScopedUserAgent]).Should(Equal(
				keySet{
					m: map[storageKey]struct{}{
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/services/endpoints/default/nginx",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/services/endpoints/kube-system/kube-dns",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/endpointslices/default/nginx",
						}: {},
						{
							comp: coordinatorconstants.DefaultPoolScopedUserAgent,
							path: "/registry/endpointslices/kube-system/kube-dns",
						}: {},
					},
				},
			))
		})
	})

	It("should recover when cache file exists and contains valid data", func() {
		Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte(
			"kubelet:/registry/pods/default/pod1,/registry/pods/default/pod2\n"+
				"kube-proxy:/registry/configmaps/kube-system/kube-proxy",
		))).To(BeNil())
		Expect(cache.Recover()).To(BeNil())
		Expect(cache.cache).To(Equal(map[string]keySet{
			"kubelet": {
				m: map[storageKey]struct{}{
					{
						comp: "kubelet",
						path: "/registry/pods/default/pod1",
					}: {},
					{
						comp: "kubelet",
						path: "/registry/pods/default/pod2",
					}: {},
				},
			},
			"kube-proxy": {
				m: map[storageKey]struct{}{
					{
						comp: "kube-proxy",
						path: "/registry/configmaps/kube-system/kube-proxy",
					}: {},
				},
			},
			coordinatorconstants.DefaultPoolScopedUserAgent: {
				m: map[storageKey]struct{}{},
			},
		}))
	})

	It("should return err when cache file contains invalid data", func() {
		Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte(
			"kubelet,/registry/pods/default/pod1",
		))).To(BeNil())
		Expect(cache.Recover()).NotTo(BeNil())
	})
})

var _ = Describe("Test componentKeyCache function", func() {
	var cache *componentKeyCache
	var fileName string
	var key1, key2, key3 storageKey
	BeforeEach(func() {
		kv := etcdmock.KV{}
		kv.On("Get", "/registry/services/endpoints", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
			Return(&clientv3.GetResponse{})
		kv.On("Get", "/registry/endpointslices", mock.AnythingOfType("clientv3.OpOption"), mock.AnythingOfType("clientv3.OpOption")).
			Return(&clientv3.GetResponse{})
		mockedClient := &clientv3.Client{KV: kv}
		etcdStorage := etcdStorage{prefix: "/registry"}
		fileName = uuid.New().String()
		cache = &componentKeyCache{
			ctx:        context.Background(),
			filePath:   filepath.Join(keyCacheDir, fileName),
			cache:      map[string]keySet{},
			fsOperator: fs.FileSystemOperator{},
			etcdClient: mockedClient,
			keyFunc:    etcdStorage.KeyFunc,
		}
		key1 = storageKey{
			path: "/registry/pods/default/pod1",
		}
		key2 = storageKey{
			path: "/registry/pods/default/pod2",
		}
		key3 = storageKey{
			path: "/registry/pods/kube-system/kube-proxy",
		}
	})
	AfterEach(func() {
		Expect(os.RemoveAll(filepath.Join(keyCacheDir, fileName))).To(BeNil())
	})

	Context("Test Load", func() {
		BeforeEach(func() {
			cache.Recover()
			cache.cache = map[string]keySet{
				"kubelet": {
					m: map[storageKey]struct{}{
						key1: {},
						key2: {},
					},
				},
			}
			cache.flush()
		})
		It("should return nil,false if component is not in cache", func() {
			c, found := cache.Load("kube-proxy")
			Expect(c.m).To(BeNil())
			Expect(found).To(BeFalse())
		})
		It("should return keyset,true if component is in cache", func() {
			c, found := cache.Load("kubelet")
			Expect(c.m).To(Equal(map[storageKey]struct{}{
				key1: {},
				key2: {},
			}))
			Expect(found).To(BeTrue())
		})
	})

	Context("Test LoadAndDelete", func() {
		BeforeEach(func() {
			cache.Recover()
			cache.cache = map[string]keySet{
				"kubelet": {
					m: map[storageKey]struct{}{
						key1: {},
						key2: {},
					},
				},
				"kube-proxy": {
					m: map[storageKey]struct{}{
						key3: {},
					},
				},
			}
			cache.flush()
		})
		It("should return nil,false if component is not in cache", func() {
			c, found := cache.LoadAndDelete("foo")
			Expect(c.m).To(BeNil())
			Expect(found).To(BeFalse())
		})
		It("should return keyset,true and delete cache for this component if exists", func() {
			c, found := cache.LoadAndDelete("kubelet")
			Expect(c.m).To(Equal(map[storageKey]struct{}{
				key1: {},
				key2: {},
			}))
			Expect(found).To(BeTrue())
			Expect(cache.cache).To(Equal(map[string]keySet{
				"kube-proxy": {
					m: map[storageKey]struct{}{
						key3: {},
					},
				},
			}))
			data, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(data).To(Equal([]byte(
				"kube-proxy:" + key3.path,
			)))
		})
	})
	Context("Test LoadOrStore", func() {
		BeforeEach(func() {
			cache.Recover()
			cache.cache = map[string]keySet{
				"kubelet": {
					m: map[storageKey]struct{}{
						key1: {},
						key2: {},
					},
				},
			}
			cache.flush()
		})
		It("should return data,false and store data if component currently does not in cache", func() {
			c, found := cache.LoadOrStore("kube-proxy", keySet{
				m: map[storageKey]struct{}{
					key3: {},
				},
			})
			Expect(found).To(BeFalse())
			Expect(c.m).To(Equal(map[storageKey]struct{}{
				key3: {},
			}))
			buf, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(strings.Split(string(buf), "\n")).To(HaveLen(2))
		})
		It("should return original data and true if component already exists in cache", func() {
			c, found := cache.LoadOrStore("kubelet", keySet{
				m: map[storageKey]struct{}{
					key3: {},
				},
			})
			Expect(found).To(BeTrue())
			Expect(c.m).To(Equal(map[storageKey]struct{}{
				key1: {},
				key2: {},
			}))
			buf, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(strings.Split(string(buf), "\n")).To(HaveLen(1))
		})
	})
})

func TestKeySetDifference(t *testing.T) {
	key1 := storageKey{path: "/registry/pods/test/test-pod"}
	key2 := storageKey{path: "/registry/pods/test/test-pod2"}
	key3 := storageKey{path: "/registry/pods/test/test-pod3"}
	cases := []struct {
		description string
		s1          keySet
		s2          keySet
		want        map[storageKey]struct{}
	}{
		{
			description: "s2 is nil",
			s1: keySet{
				m: map[storageKey]struct{}{
					key1: {},
					key2: {},
				},
			},
			s2: keySet{
				m: nil,
			},
			want: map[storageKey]struct{}{
				key1: {},
				key2: {},
			},
		}, {
			description: "s2 is empty",
			s1: keySet{
				m: map[storageKey]struct{}{
					key1: {},
					key2: {},
				},
			},
			s2: keySet{
				m: map[storageKey]struct{}{},
			},
			want: map[storageKey]struct{}{
				key1: {},
				key2: {},
			},
		},
		{
			description: "s1 is empty",
			s1: keySet{
				m: map[storageKey]struct{}{},
			},
			s2: keySet{
				m: map[storageKey]struct{}{
					key1: {},
					key2: {},
				},
			},
			want: map[storageKey]struct{}{},
		},
		{
			description: "s1 has intersection with s2",
			s1: keySet{
				m: map[storageKey]struct{}{
					key1: {},
					key2: {},
				},
			},
			s2: keySet{
				m: map[storageKey]struct{}{
					key2: {},
					key3: {},
				},
			},
			want: map[storageKey]struct{}{
				key1: {},
			},
		},
	}

	for _, c := range cases {
		got := c.s1.Difference(c.s2)
		if len(got) != len(c.want) {
			t.Errorf("unexpected num of keys at case %s, got: %d, want: %d", c.description, len(got), len(c.want))
		}
		gotm := map[storageKey]struct{}{}
		for _, k := range got {
			gotm[k] = struct{}{}
		}

		if !reflect.DeepEqual(gotm, c.want) {
			t.Errorf("failed at case %s, got: %v, want: %v", c.description, got, c.want)
		}
	}
}
