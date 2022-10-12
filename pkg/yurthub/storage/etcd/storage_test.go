package etcd

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

var _ = Describe("Test EtcdStorage", func() {
	var etcdstore *etcdStorage
	var key1 storage.Key
	var podObj *v1.Pod
	var podJson []byte
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
		randomize := uuid.New().String()
		cfg := &EtcdStorageConfig{
			Prefix:        "/" + randomize,
			EtcdEndpoints: []string{"127.0.0.1:2379"},
			LocalCacheDir: filepath.Join(keyCacheDir, randomize),
		}
		s, err := NewStorage(context.Background(), cfg)
		Expect(err).To(BeNil())
		etcdstore = s.(*etcdStorage)
		key1, err = etcdstore.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Group:     "",
			Version:   "v1",
			Namespace: "default",
			Name:      "pod1-" + randomize,
		})
		Expect(err).To(BeNil())
		podObj = &v1.Pod{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "pod1-" + randomize,
				Namespace:       "default",
				ResourceVersion: "890",
			},
		}
		podJson, err = json.Marshal(podObj)
		Expect(err).To(BeNil())
	})

	Context("Test Create", func() {
		It("should return ErrKeyIsEmpty if key is nil", func() {
			Expect(etcdstore.Create(nil, []byte("foo"))).To(Equal(storage.ErrKeyIsEmpty))
			Expect(etcdstore.Create(storageKey{}, []byte("foo"))).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrKeyHasNoContent if content is empty", func() {
			Expect(etcdstore.Create(key1, []byte{})).To(Equal(storage.ErrKeyHasNoContent))
			Expect(etcdstore.Create(key1, nil)).To(Equal(storage.ErrKeyHasNoContent))
		})
		It("should return ErrKeyExists if key already exists in etcd", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			Expect(etcdstore.Create(key1, podJson)).To(Equal(storage.ErrKeyExists))
		})
		It("should create key with content in etcd", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			resp, err := etcdstore.client.Get(ctx, key1.Key())
			Expect(err).To(BeNil())
			Expect(resp.Kvs).To(HaveLen(1))
			Expect(resp.Kvs[0].Value).To(Equal([]byte(podJson)))
			resp, err = etcdstore.client.Get(ctx, etcdstore.mirrorPath(key1.Key(), rvType))
			Expect(err).To(BeNil())
			Expect(resp.Kvs).To(HaveLen(1))
			Expect(resp.Kvs[0].Value).To(Equal([]byte(fixLenRvString(podObj.ResourceVersion))))
		})
	})

	Context("Test Delete", func() {
		It("should return ErrKeyIsEmpty if key is nil", func() {
			Expect(etcdstore.Delete(nil)).To(Equal(storage.ErrKeyIsEmpty))
			Expect(etcdstore.Delete(storageKey{})).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should delete key from etcd if it exists", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			Expect(etcdstore.Delete(key1)).To(BeNil())
			resp, err := etcdstore.client.Get(ctx, key1.Key())
			Expect(err).To(BeNil())
			Expect(resp.Kvs).To(HaveLen(0))
			resp, err = etcdstore.client.Get(ctx, etcdstore.mirrorPath(key1.Key(), rvType))
			Expect(err).To(BeNil())
			Expect(resp.Kvs).To(HaveLen(0))
		})
		It("should not return error if key does not exist in etcd", func() {
			Expect(etcdstore.Delete(key1)).To(BeNil())
		})
	})

	Context("Test Get", func() {
		It("should return ErrKeyIsEmpty if key is nil", func() {
			_, err := etcdstore.Get(nil)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
			_, err = etcdstore.Get(storageKey{})
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if key does not exist in etcd", func() {
			_, err := etcdstore.Get(key1)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return content of key if it exists in etcd", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			buf, err := etcdstore.Get(key1)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(podJson))
		})
	})

	Context("Test List", func() {
		var err error
		var podsRootKey storage.Key
		BeforeEach(func() {
			podsRootKey, err = etcdstore.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
			})
		})
		It("should return ErrKeyIsEmpty if key is nil", func() {
			_, err = etcdstore.List(nil)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
			_, err = etcdstore.List(storageKey{})
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if key does not exist in etcd", func() {
			_, err = etcdstore.List(podsRootKey)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
			_, err = etcdstore.List(key1)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return a single resource if key points to a specific resource", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			buf, err := etcdstore.List(key1)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([][]byte{podJson}))
		})
		It("should return a list of resources if its is a root key", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			info := storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Group:     "",
				Version:   "v1",
				Namespace: "default",
			}

			info.Name = "pod2"
			key2, err := etcdstore.KeyFunc(info)
			Expect(err).To(BeNil())
			pod2Obj := podObj.DeepCopy()
			pod2Obj.Name = "pod2"
			pod2Json, err := json.Marshal(pod2Obj)
			Expect(err).To(BeNil())
			Expect(etcdstore.Create(key2, pod2Json)).To(BeNil())

			info.Name = "pod3"
			info.Namespace = "kube-system"
			key3, err := etcdstore.KeyFunc(info)
			Expect(err).To(BeNil())
			pod3Obj := podObj.DeepCopy()
			pod3Obj.Name = "pod3"
			pod3Obj.Namespace = "kube-system"
			pod3Json, err := json.Marshal(pod3Obj)
			Expect(err).To(BeNil())
			Expect(etcdstore.Create(key3, pod3Json)).To(BeNil())

			buf, err := etcdstore.List(podsRootKey)
			Expect(err).To(BeNil())
			Expect(buf).To(HaveLen(len(buf)))
			Expect(buf).To(ContainElements([][]byte{podJson, pod2Json, pod3Json}))

			namespacesRootKey, _ := etcdstore.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
			})
			buf, err = etcdstore.List(namespacesRootKey)
			Expect(err).To(BeNil())
			Expect(buf).To(ContainElements([][]byte{podJson, pod2Json}))
		})
	})

	Context("Test Update", func() {
		It("should return ErrKeyIsEmpty if key is nil", func() {
			_, err := etcdstore.Update(nil, []byte("foo"), 100)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
			_, err = etcdstore.Update(storageKey{}, []byte("foo"), 100)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if key does not exist in etcd", func() {
			_, err := etcdstore.Update(key1, podJson, 890)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return resource in etcd and ErrUpdateConflict if the provided resource has staler rv than resource in etcd", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())
			podObj.ResourceVersion = "100"
			podObj.Labels = map[string]string{
				"new": "label",
			}
			newPodJson, err := json.Marshal(podObj)
			Expect(err).To(BeNil())
			stored, err := etcdstore.Update(key1, newPodJson, 100)
			Expect(err).To(Equal(storage.ErrUpdateConflict))
			Expect(stored).To(Equal(podJson))
		})
		It("should update resource in etcd and return the stored resource", func() {
			Expect(etcdstore.Create(key1, podJson)).To(BeNil())

			podObj.ResourceVersion = "900"
			podObj.Labels = map[string]string{
				"rv": "900",
			}
			newPodJson, err := json.Marshal(podObj)
			Expect(err).To(BeNil())
			stored, err := etcdstore.Update(key1, newPodJson, 900)
			Expect(err).To(BeNil())
			Expect(stored).To(Equal(newPodJson))

			podObj.ResourceVersion = "1000"
			podObj.Labels = map[string]string{
				"rv": "1000",
			}
			newPodJson, err = json.Marshal(podObj)
			Expect(err).To(BeNil())
			stored, err = etcdstore.Update(key1, newPodJson, 1000)
			Expect(err).To(BeNil())
			Expect(stored).To(Equal(newPodJson))
		})
	})

	Context("Test ListResourceKeysOfComponent", func() {

	})

	Context("Test ReplaceComponentList", func() {

	})

	Context("Test DeleteComponentResources", func() {

	})
})
