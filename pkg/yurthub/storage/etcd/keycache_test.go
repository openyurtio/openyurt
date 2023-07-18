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
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	etcdmock "github.com/openyurtio/openyurt/pkg/yurthub/storage/etcd/mock"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
	coordinatorconstants "github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/constants"
)

var (
	podGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}
	endpointSliceGVR = schema.GroupVersionResource{
		Group:    "discovery.k8s.io",
		Version:  "v1",
		Resource: "endpointslices",
	}
	endpointGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "endpoints",
	}
	cmGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	svcGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}
)

var _ = Describe("Test componentKeyCache setup", func() {
	var cache *componentKeyCache
	var fileName string
	var f fs.FileSystemOperator
	var mockedClient *clientv3.Client
	BeforeEach(func() {
		kv := &etcdmock.KV{}
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
			cache:      map[string]keyCache{},
			fsOperator: fs.FileSystemOperator{},
			etcdClient: mockedClient,
			keyFunc:    etcdStorage.KeyFunc,
			poolScopedResourcesGetter: func() []schema.GroupVersionResource {
				return []schema.GroupVersionResource{
					endpointGVR, endpointSliceGVR,
				}
			},
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
			kv := &etcdmock.KV{}
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
				keyCache{
					m: map[schema.GroupVersionResource]storageKeySet{
						endpointGVR: {
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/default/nginx",
							}: {},
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/kube-system/kube-dns",
							}: {},
						},
						endpointSliceGVR: {
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointSliceGVR,
								path: "/registry/endpointslices/default/nginx",
							}: {},
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointSliceGVR,
								path: "/registry/endpointslices/kube-system/kube-dns",
							}: {},
						},
					},
				},
			))
		})

		It("should replace leader-yurthub cache read from local file with keys from etcd", func() {
			Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte(
				"leader-yurthub#_v1_endpoints:/registry/services/endpoints/default/nginx-local,"+
					"/registry/services/endpoints/kube-system/kube-dns-local;"+
					"discovery.k8s.io_v1_endpointslices:/registry/endpointslices/default/nginx-local,"+
					"/registry/endpointslices/kube-system/kube-dns-local",
			))).To(BeNil())
			Expect(cache.Recover()).To(BeNil())
			Expect(cache.cache[coordinatorconstants.DefaultPoolScopedUserAgent]).Should(Equal(
				keyCache{
					m: map[schema.GroupVersionResource]storageKeySet{
						endpointGVR: {
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/default/nginx",
							}: {},
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/kube-system/kube-dns",
							}: {},
						},
						endpointSliceGVR: {
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointSliceGVR,
								path: "/registry/endpointslices/default/nginx",
							}: {},
							{
								comp: coordinatorconstants.DefaultPoolScopedUserAgent,
								gvr:  endpointSliceGVR,
								path: "/registry/endpointslices/kube-system/kube-dns",
							}: {},
						},
					},
				},
			))
		})
	})

	It("should recover when cache file exists and contains valid data", func() {
		Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte(
			"kubelet#_v1_pods:/registry/pods/default/pod1,/registry/pods/default/pod2\n"+
				"kube-proxy#_v1_configmaps:/registry/configmaps/kube-system/kube-proxy",
		))).To(BeNil())
		Expect(cache.Recover()).To(BeNil())
		Expect(cache.cache).To(Equal(map[string]keyCache{
			"kubelet": {
				m: map[schema.GroupVersionResource]storageKeySet{
					podGVR: {
						{
							comp: "kubelet",
							gvr:  podGVR,
							path: "/registry/pods/default/pod1",
						}: {},
						{
							comp: "kubelet",
							gvr:  podGVR,
							path: "/registry/pods/default/pod2",
						}: {},
					},
				},
			},
			"kube-proxy": {
				m: map[schema.GroupVersionResource]storageKeySet{
					cmGVR: {
						{
							comp: "kube-proxy",
							gvr:  cmGVR,
							path: "/registry/configmaps/kube-system/kube-proxy",
						}: {},
					},
				},
			},
			coordinatorconstants.DefaultPoolScopedUserAgent: {
				m: map[schema.GroupVersionResource]storageKeySet{},
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
		kv := &etcdmock.KV{}
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
			cache:      map[string]keyCache{},
			fsOperator: fs.FileSystemOperator{},
			etcdClient: mockedClient,
			keyFunc:    etcdStorage.KeyFunc,
			poolScopedResourcesGetter: func() []schema.GroupVersionResource {
				return []schema.GroupVersionResource{
					endpointGVR, endpointSliceGVR,
				}
			},
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
			cache.cache = map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {
							key1: {},
							key2: {},
						},
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
			Expect(c.m).To(Equal(map[schema.GroupVersionResource]storageKeySet{
				podGVR: {
					key1: {},
					key2: {},
				},
			}))
			Expect(found).To(BeTrue())
		})
	})

	Context("Test LoadAndDelete", func() {
		BeforeEach(func() {
			cache.Recover()
			cache.cache = map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {
							key1: {},
							key2: {},
						},
					},
				},
				"kube-proxy": {
					m: map[schema.GroupVersionResource]storageKeySet{
						cmGVR: {
							key3: {},
						},
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
			Expect(c.m).To(Equal(map[schema.GroupVersionResource]storageKeySet{
				podGVR: {
					key1: {},
					key2: {},
				},
			}))
			Expect(found).To(BeTrue())
			Expect(cache.cache).To(Equal(map[string]keyCache{
				"kube-proxy": {
					m: map[schema.GroupVersionResource]storageKeySet{
						cmGVR: {
							key3: {},
						},
					},
				},
			}))
			data, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(data).To(Equal([]byte(
				"kube-proxy#_v1_configmaps:" + key3.path,
			)))
		})
	})
	Context("Test LoadOrStore", func() {
		BeforeEach(func() {
			cache.Recover()
			cache.cache = map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {
							key1: {},
							key2: {},
						},
					},
				},
			}
			cache.flush()
		})
		It("should return data,false and store data if component currently does not in cache", func() {
			c, found := cache.LoadOrStore("kube-proxy", cmGVR, storageKeySet{key3: {}})
			Expect(found).To(BeFalse())
			Expect(c).To(Equal(storageKeySet{key3: {}}))
			buf, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(strings.Split(string(buf), "\n")).To(HaveLen(2))
		})
		It("should return original data and true if component already exists in cache", func() {
			c, found := cache.LoadOrStore("kubelet", podGVR, storageKeySet{key3: {}})
			Expect(found).To(BeTrue())
			Expect(c).To(Equal(storageKeySet{
				key1: {},
				key2: {},
			}))
			buf, err := os.ReadFile(cache.filePath)
			Expect(err).To(BeNil())
			Expect(strings.Split(string(buf), "\n")).To(HaveLen(1))
		})
	})
})

func TestMarshal(t *testing.T) {
	cases := []struct {
		description string
		cache       map[string]keyCache
		want        []byte
	}{
		{
			description: "cache is nil",
			cache:       map[string]keyCache{},
			want:        []byte{},
		},
		{
			description: "component has empty cache",
			cache: map[string]keyCache{
				"kubelet":    {m: map[schema.GroupVersionResource]storageKeySet{}},
				"kube-proxy": {m: map[schema.GroupVersionResource]storageKeySet{}},
			},
		},
		{
			description: "empty gvr keySet",
			cache: map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {},
					},
				},
			},
		},
		{
			description: "marshal cache with keys",
			cache: map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {
							{
								comp: "kubelet",
								gvr:  podGVR,
								path: "/registry/pods/default/nginx",
							}: struct{}{},
							{
								comp: "kubelet",
								gvr:  podGVR,
								path: "/registry/pods/kube-system/kube-proxy",
							}: struct{}{},
						},
						cmGVR: {
							{
								comp: "kubelet",
								gvr:  cmGVR,
								path: "/registry/configmaps/kube-system/coredns",
							}: struct{}{},
						},
					},
				},
				"kube-proxy": {
					m: map[schema.GroupVersionResource]storageKeySet{
						endpointGVR: {
							{
								comp: "kube-proxy",
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/kube-system/kube-dns",
							}: {},
							{
								comp: "kube-proxy",
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/default/kubernetes",
							}: {},
						},
						endpointSliceGVR: {
							{
								comp: "kube-proxy",
								gvr:  endpointSliceGVR,
								path: "/registry/discovery.k8s.io/endpointslices/kube-system/kube-dns",
							}: {},
							{
								comp: "kube-proxy",
								gvr:  endpointSliceGVR,
								path: "/registry/discovery.k8s.io/endpointslices/default/kubernetes",
							}: {},
						},
						svcGVR: {
							{
								comp: "kube-proxy",
								gvr:  svcGVR,
								path: "/registry/services/specs/kube-system/kube-dns",
							}: {},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			buf := marshal(c.cache)
			if c.want != nil && !reflect.DeepEqual(buf, c.want) {
				t.Errorf("unexpected result want: %s, got: %s", c.want, buf)
			}
			cache, err := unmarshal(buf)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cache, c.cache) {
				t.Errorf("unexpected cache, want: %v, got: %v", c.cache, cache)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	cases := []struct {
		description string
		content     string
		want        map[string]keyCache
		wantErr     bool
	}{
		{
			description: "empty content",
			content:     "",
			want:        map[string]keyCache{},
		},
		{
			description: "components have empty keyCache",
			content: "kubelet#\n" +
				"kube-proxy#",
			want: map[string]keyCache{
				"kubelet":    {m: map[schema.GroupVersionResource]storageKeySet{}},
				"kube-proxy": {m: map[schema.GroupVersionResource]storageKeySet{}},
			},
		},
		{
			description: "invalid component format",
			content: "kubelet\n" +
				"kube-proxy",
			wantErr: true,
		},
		{
			description: "gvr of component has empty keySet",
			content: "kubelet#\n" +
				"kube-proxy#_v1_endpoints:;discovery.k8s.io_v1_endpointslices:",
			want: map[string]keyCache{
				"kubelet": {m: map[schema.GroupVersionResource]storageKeySet{}},
				"kube-proxy": {
					m: map[schema.GroupVersionResource]storageKeySet{
						endpointGVR:      {},
						endpointSliceGVR: {},
					},
				},
			},
		},
		{
			description: "invalid gvr format that do not have suffix colon",
			content:     "kubelet#_v1_pods",
			wantErr:     true,
		},
		{
			description: "invalid gvr format that uses unrecognized separator",
			content:     "kubelet#.v1.pods",
			wantErr:     true,
		},
		{
			description: "unmarshal keys and generate cache",
			content: "kubelet#_v1_pods:/registry/pods/default/nginx,/registry/pods/kube-system/kube-proxy\n" +
				"kube-proxy#discovery.k8s.io_v1_endpointslices:/registry/endpointslices/kube-system/kube-dns;" +
				"_v1_endpoints:/registry/services/endpoints/kube-system/kube-dns",
			want: map[string]keyCache{
				"kubelet": {
					m: map[schema.GroupVersionResource]storageKeySet{
						podGVR: {
							{
								comp: "kubelet",
								gvr:  podGVR,
								path: "/registry/pods/default/nginx",
							}: {},
							{
								comp: "kubelet",
								gvr:  podGVR,
								path: "/registry/pods/kube-system/kube-proxy",
							}: {},
						},
					},
				},
				"kube-proxy": {
					m: map[schema.GroupVersionResource]storageKeySet{
						endpointGVR: {
							{
								comp: "kube-proxy",
								gvr:  endpointGVR,
								path: "/registry/services/endpoints/kube-system/kube-dns",
							}: {},
						},
						endpointSliceGVR: {
							{
								comp: "kube-proxy",
								gvr:  endpointSliceGVR,
								path: "/registry/endpointslices/kube-system/kube-dns",
							}: {},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			cache, err := unmarshal([]byte(c.content))
			if (c.wantErr && err == nil) || (!c.wantErr && err != nil) {
				t.Errorf("unexpected err, if want error: %v, got: %v", c.wantErr, err)
			}

			if err != nil {
				return
			}

			if !reflect.DeepEqual(cache, c.want) {
				t.Errorf("unexpected cache, want: %v, got: %v", c.want, cache)
			}
		})
	}
}

func TestStorageKeySetDifference(t *testing.T) {
	podKey1 := storageKey{path: "/registry/pods/test/test-pod"}
	podKey2 := storageKey{path: "/registry/pods/test/test-pod2"}
	podKey3 := storageKey{path: "/registry/pods/test/test-pod3"}
	cases := []struct {
		description string
		s1          storageKeySet
		s2          storageKeySet
		gvr         schema.GroupVersionResource
		want        storageKeySet
	}{
		{
			description: "s2 is nil",
			s1: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
			s2:  nil,
			gvr: podGVR,
			want: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
		}, {
			description: "s2 is empty",
			s1: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
			s2:  storageKeySet{},
			gvr: podGVR,
			want: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
		},
		{
			description: "s1 is empty",
			s1:          storageKeySet{},
			s2: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
			gvr:  podGVR,
			want: storageKeySet{},
		},
		{
			description: "s1 has intersection with s2",
			s1: storageKeySet{
				podKey1: {},
				podKey2: {},
			},
			s2: storageKeySet{
				podKey2: {},
				podKey3: {},
			},
			want: map[storageKey]struct{}{
				podKey1: {},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			got := c.s1.Difference(c.s2)
			if len(got) != len(c.want) {
				t.Errorf("unexpected num of keys at case %s, got: %d, want: %d", c.description, len(got), len(c.want))
			}

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("failed at case %s, got: %v, want: %v", c.description, got, c.want)
			}
		})
	}
}
