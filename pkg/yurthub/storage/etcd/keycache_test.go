package etcd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

var _ = Describe("Test componentKeyCache setup", func() {
	var cache *componentKeyCache
	var fileName string
	var f fs.FileSystemOperator
	BeforeEach(func() {
		fileName = uuid.New().String()
		cache = newComponentKeyCache(filepath.Join(keyCacheDir, fileName))
	})
	AfterEach(func() {
		Expect(os.RemoveAll(filepath.Join(keyCacheDir, fileName)))
	})

	It("should recover when cache file does not exist", func() {
		Expect(cache.Recover()).To(BeNil())
		Expect(len(cache.cache)).To(BeZero())
	})

	It("should recover when cache file is empty", func() {
		Expect(f.CreateFile(filepath.Join(keyCacheDir, fileName), []byte{})).To(BeNil())
		Expect(cache.Recover()).To(BeNil())
		Expect(len(cache.cache)).To(BeZero())
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
					{path: "/registry/pods/default/pod1"}: {},
					{path: "/registry/pods/default/pod2"}: {},
				},
			},
			"kube-proxy": {
				m: map[storageKey]struct{}{
					{path: "/registry/configmaps/kube-system/kube-proxy"}: {},
				},
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
		fileName = uuid.New().String()
		cache = newComponentKeyCache(filepath.Join(keyCacheDir, fileName))
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
