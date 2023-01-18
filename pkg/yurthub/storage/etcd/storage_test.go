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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

// TODO: These tests should be integration tests instead of unit tests.
// Currently, we will install the etcd cmd BeforeSuite to make these tests work around.
// But they are better moved to integration test dir.
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
			UnSecure:      true,
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

	Context("Test Lifecycle", func() {
		It("should reconnect to etcd if connect once broken", func() {
			Expect(etcdstore.Create(key1, podJson)).Should(BeNil())
			Expect(etcdCmd.Process.Kill()).To(BeNil())
			By("waiting for the etcd exited")
			Eventually(func() bool {
				_, err := etcdstore.Get(key1)
				return err != nil
			}, 10*time.Second, 1*time.Second).Should(BeTrue())

			devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0755)
			Expect(err).To(BeNil())
			etcdCmd = exec.Command(etcdCmdPath, "--data-dir="+etcdDataDir)
			etcdCmd.Stdout = devNull
			etcdCmd.Stderr = devNull
			Expect(etcdCmd.Start()).To(BeNil())
			By("waiting for storage function recovery")
			Eventually(func() bool {
				if _, err := etcdstore.Get(key1); err != nil {
					return false
				}
				return true
			}, 30*time.Second, 500*time.Microsecond).Should(BeTrue())
		})
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

	Context("Test ComponentRelatedInterface", func() {
		var cmKey, key2, key3 storage.Key
		var cmObj *v1.ConfigMap
		var pod2Obj, pod3Obj *v1.Pod
		var cmJson, pod2Json, pod3Json []byte
		var gvr schema.GroupVersionResource
		var err error
		BeforeEach(func() {
			info := storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Group:     "",
				Version:   "v1",
			}
			gvr = schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			}

			info.Namespace = "default"
			info.Name = "pod2"
			key2, _ = etcdstore.KeyFunc(info)
			info.Namespace = "kube-system"
			info.Name = "pod3"
			key3, _ = etcdstore.KeyFunc(info)
			cmKey, _ = etcdstore.KeyFunc(storage.KeyBuildInfo{
				Group:     "",
				Resources: "configmaps",
				Version:   "v1",
				Namespace: "default",
				Name:      "cm",
				Component: "kubelet",
			})

			pod2Obj = podObj.DeepCopy()
			pod2Obj.Namespace = "default"
			pod2Obj.Name = "pod2"
			pod2Obj.ResourceVersion = "920"
			pod3Obj = podObj.DeepCopy()
			pod3Obj.Namespace = "kube-system"
			pod3Obj.Name = "pod3"
			pod3Obj.ResourceVersion = "930"
			cmObj = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm",
					Namespace: "default",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "v1",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			}

			pod2Json, err = json.Marshal(pod2Obj)
			Expect(err).To(BeNil())
			pod3Json, err = json.Marshal(pod3Obj)
			Expect(err).To(BeNil())
			cmJson, err = json.Marshal(cmObj)
			Expect(err).To(BeNil())
		})
		Context("Test ListResourceKeysOfComponent", func() {
			It("should return ErrEmptyComponent if component is empty", func() {
				_, err = etcdstore.ListResourceKeysOfComponent("", gvr)
				Expect(err).To(Equal(storage.ErrEmptyComponent))
			})
			It("should return ErrEmptyResource if resource of gvr is empty", func() {
				_, err = etcdstore.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Resource: "",
					Version:  "v1",
					Group:    "",
				})
				Expect(err).To(Equal(storage.ErrEmptyResource))
			})
			It("should return ErrStorageNotFound if this component has no cache", func() {
				_, err = etcdstore.ListResourceKeysOfComponent("flannel", gvr)
				Expect(err).To(Equal(storage.ErrStorageNotFound))
			})
			It("should return all keys of gvr if cache of this component is found", func() {
				By("creating objects in cache")
				Expect(etcdstore.Create(key1, podJson)).To(BeNil())
				Expect(etcdstore.Create(key3, pod3Json)).To(BeNil())
				Expect(etcdstore.Create(cmKey, cmJson)).To(BeNil())

				keys, err := etcdstore.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "pods",
				})
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(2))
				Expect(keys).To(ContainElements(key1, key3))
			})
		})

		Context("Test ReplaceComponentList", func() {
			BeforeEach(func() {
				Expect(etcdstore.Create(key1, podJson)).To(BeNil())
				Expect(etcdstore.Create(key2, pod2Json)).To(BeNil())
				Expect(etcdstore.Create(key3, pod3Json)).To(BeNil())
			})
			It("should return ErrEmptyComponent if component is empty", func() {
				Expect(etcdstore.ReplaceComponentList("", gvr, "", map[storage.Key][]byte{})).To(Equal(storage.ErrEmptyComponent))
			})
			It("should return ErrEmptyResource if resource of gvr is empty", func() {
				gvr.Resource = ""
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "", map[storage.Key][]byte{})).To(Equal(storage.ErrEmptyResource))
			})
			It("should return ErrInvalidContent if it exists keys that are not passed-in gvr", func() {
				invalidKey, err := etcdstore.KeyFunc(storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "configmaps",
					Group:     "",
					Version:   "v1",
					Namespace: "default",
					Name:      "cm",
				})
				Expect(err).To(BeNil())
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "default", map[storage.Key][]byte{
					key2:       pod2Json,
					invalidKey: {},
				})).To(Equal(storage.ErrInvalidContent))

				invalidKey, err = etcdstore.KeyFunc(storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Group:     "",
					Version:   "v1",
					Namespace: "kube-system",
					Name:      "pod4",
				})
				Expect(err).To(BeNil())
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "default", map[storage.Key][]byte{
					key2:       pod2Json,
					key3:       pod3Json,
					invalidKey: {},
				})).To(Equal(storage.ErrInvalidContent))
			})
			It("should only use fresher resources in contents to update cache in etcd", func() {
				pod2Obj.ResourceVersion = "921"
				newPod2Json, err := json.Marshal(pod2Obj)
				Expect(err).To(BeNil())
				pod3Obj.ResourceVersion = "1001" // case of different len(ResourceVersion)
				newPod3Json, err := json.Marshal(pod3Obj)
				Expect(err).To(BeNil())
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "", map[storage.Key][]byte{
					key1: podJson,
					key2: newPod2Json,
					key3: newPod3Json,
				})).To(BeNil())

				buf, err := etcdstore.Get(key1)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(podJson))
				buf, err = etcdstore.Get(key2)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(newPod2Json))
				buf, err = etcdstore.Get(key3)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(newPod3Json))
			})
			It("should create resource if it does not in etcd", func() {
				key4, _ := etcdstore.KeyFunc(storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Version:   "v1",
					Group:     "",
					Namespace: "default",
					Name:      "pod4",
				})
				pod4Obj := podObj.DeepCopy()
				pod4Obj.ResourceVersion = "940"
				pod4Obj.Name = "pod4"
				pod4Json, err := json.Marshal(pod4Obj)
				Expect(err).To(BeNil())
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "", map[storage.Key][]byte{
					key1: podJson,
					key2: pod2Json,
					key3: pod3Json,
					key4: pod4Json,
				})).To(BeNil())

				buf, err := etcdstore.Get(key1)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(podJson))
				buf, err = etcdstore.Get(key2)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(pod2Json))
				buf, err = etcdstore.Get(key3)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(pod3Json))
				buf, err = etcdstore.Get(key4)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(pod4Json))
			})
			It("should delete resources in etcd if they were in local cache but are not in current contents", func() {
				Expect(etcdstore.Create(cmKey, cmJson)).Should(BeNil())
				Expect(etcdstore.ReplaceComponentList("kubelet", gvr, "", map[storage.Key][]byte{
					key1: podJson,
				})).To(BeNil())
				buf, err := etcdstore.Get(key1)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(podJson))
				_, err = etcdstore.Get(key2)
				Expect(err).To(Equal(storage.ErrStorageNotFound))
				_, err = etcdstore.Get(key3)
				Expect(err).To(Equal(storage.ErrStorageNotFound))

				// Should not delete resources of other gvr
				buf, err = etcdstore.Get(cmKey)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(cmJson))
			})
		})

		Context("Test DeleteComponentResources", func() {
			It("should return ErrEmptyComponent if component is empty", func() {
				Expect(etcdstore.DeleteComponentResources("")).To(Equal(storage.ErrEmptyComponent))
			})
			It("should not return err even there is no cache of component", func() {
				Expect(etcdstore.DeleteComponentResources("flannel")).To(BeNil())
			})
			It("should delete cache of component from local cache and etcd", func() {
				Expect(etcdstore.Create(cmKey, cmJson)).To(BeNil())
				Expect(etcdstore.Create(key1, podJson)).To(BeNil())
				Expect(etcdstore.Create(key3, pod3Json)).To(BeNil())
				keys := []storage.Key{cmKey, key1, key3}
				cmKey, _ = etcdstore.KeyFunc(storage.KeyBuildInfo{
					Component: "kube-proxy",
					Resources: "configmaps",
					Group:     "",
					Version:   "v1",
					Namespace: "default",
					Name:      "cm-kube-proxy",
				})
				cmObj.Name = "cm-kube-proxy"
				cmJson, err = json.Marshal(cmObj)
				Expect(err).To(BeNil())
				Expect(etcdstore.Create(cmKey, cmJson)).To(BeNil())

				Expect(etcdstore.DeleteComponentResources("kubelet")).To(BeNil())
				for _, k := range keys {
					_, err := etcdstore.Get(k)
					Expect(err).To(Equal(storage.ErrStorageNotFound))
				}
				buf, err := etcdstore.Get(cmKey)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(cmJson))

				_, found := etcdstore.localComponentKeyCache.Load("kubelet")
				Expect(found).To(BeFalse())
				cache, found := etcdstore.localComponentKeyCache.Load("kube-proxy")
				Expect(found).To(BeTrue())
				Expect(cache).To(Equal(keyCache{
					m: map[schema.GroupVersionResource]storageKeySet{
						cmGVR: {
							cmKey.(storageKey): {},
						},
					},
				}))
			})
		})
	})
})
