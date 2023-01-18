/*
Copyright 2020 The OpenYurt Authors.

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

package disk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

var diskStorageTestBaseDir = "/tmp/diskStorage-funcTest"
var podObj = v1.Pod{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Pod",
	},
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"k8s-app": "yurt-tunnel-agent",
		},
		Name:            "yurt-tunnel-agent-wjx67",
		Namespace:       "kube-system",
		ResourceVersion: "890",
	},
	Spec: v1.PodSpec{
		NodeName: "openyurt-e2e-test-worker",
		NodeSelector: map[string]string{
			"beta.kubernetes.io/os":      "linux",
			"openyurt.io/is-edge-worker": "true",
		},
	},
}
var nodeObj = v1.Node{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Node",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "edge-worker",
		ResourceVersion: "100",
	},
	Spec: v1.NodeSpec{},
}

const (
	versionJSONBytes = `{
		"major": "1",
		"minor": "22",
		"gitVersion": "v1.22.7",
		"gitCommit": "b56e432f2191419647a6a13b9f5867801850f969",
		"gitTreeState": "clean",
		"buildDate": "2022-03-06T21:07:35Z",
		"goVersion": "go1.16.14",
		"compiler": "gc",
		"platform": "linux/amd64"
	  }`
)

var _ = BeforeSuite(func() {
	err := os.RemoveAll(diskStorageTestBaseDir)
	Expect(err).To(BeNil())
	err = os.MkdirAll(diskStorageTestBaseDir, 0755)
	Expect(err).To(BeNil())
})

var _ = AfterSuite(func() {
	err := os.RemoveAll(diskStorageTestBaseDir)
	Expect(err).To(BeNil())
})

var _ = Describe("Test DiskStorage Setup", func() {
	var store *diskStorage
	var baseDir string
	var err error
	var fileGenerator func(basePath string, content []byte) error
	var fileChecker func(basePath string, content []byte) error
	BeforeEach(func() {
		baseDir = filepath.Join(diskStorageTestBaseDir, uuid.New().String())
		Expect(err).To(BeNil())
		store = &diskStorage{
			baseDir:    baseDir,
			fsOperator: &fs.FileSystemOperator{},
		}
		fileChecker = func(basePath string, content []byte) error {
			cnt := 3
			for i := 0; i < cnt; i++ {
				path := fmt.Sprintf("%s/resource%d", basePath, i)
				buf, err := checkFileAt(path)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(buf, content) {
					return fmt.Errorf("wrong content at %s, want: %s, got: %s", path, string(content), string(buf))
				}
			}
			return nil
		}
		fileGenerator = func(basePath string, content []byte) error {
			cnt := 3
			if err := os.MkdirAll(basePath, 0755); err != nil {
				return err
			}
			for i := 0; i < cnt; i++ {
				path := fmt.Sprintf("%s/resource%d", basePath, i)
				if err := writeFileAt(path, content); err != nil {
					return err
				}
			}
			return nil
		}
	})
	AfterEach(func() {
		err = os.RemoveAll(baseDir)
		Expect(err).To(BeNil())
	})

	Context("Test recoverFile", func() {
		It("should recover when tmp path and origin path are both regular file", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/kube-root-ca.crt")
			err = writeFileAt(originPath, []byte("origin-data"))
			Expect(err).To(BeNil())
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/tmp_kube-root-ca.crt")
			err = writeFileAt(tmpPath, []byte("tmp-data"))
			Expect(err).To(BeNil())
			err = store.recoverFile(tmpPath)
			Expect(err).To(BeNil())
			buf, err := checkFileAt(originPath)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte("tmp-data")))
		})
		It("should recover when origin path does not exist", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/kube-root-ca.crt")
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/tmp_kube-root-ca.crt")
			err = writeFileAt(tmpPath, []byte("tmp-data"))
			Expect(err).To(BeNil())
			err = store.recoverFile(tmpPath)
			Expect(err).To(BeNil())
			buf, err := checkFileAt(originPath)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte("tmp-data")))
		})
		It("should return error if tmp path is not a regular file", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/kube-root-ca.crt")
			err = writeFileAt(originPath, []byte("origin-data"))
			Expect(err).To(BeNil())
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/tmp_kube-root-ca.crt")
			err = os.MkdirAll(tmpPath, 0755)
			Expect(err).To(BeNil())
			err = store.recoverFile(tmpPath)
			Expect(err).NotTo(BeNil())
		})
		It("should return error if origin path is not a regular file", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/kube-root-ca.crt")
			err = os.MkdirAll(originPath, 0755)
			Expect(err).To(BeNil())
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default/tmp_kube-root-ca.crt")
			err = writeFileAt(tmpPath, []byte("tmp-data"))
			Expect(err).To(BeNil())
			err = store.recoverFile(tmpPath)
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Test recoverDir", func() {
		It("should recover if tmp path and origin path are both dir", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default")
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/tmp_default")
			originData := []byte("origin")
			tmpData := []byte("tmp")
			err = fileGenerator(originPath, originData)
			Expect(err).To(BeNil())
			err = fileGenerator(tmpPath, tmpData)
			Expect(err).To(BeNil())
			err = store.recoverDir(tmpPath)
			Expect(err).To(BeNil())
			Expect(fs.IfExists(tmpPath)).To(BeFalse())
			err = fileChecker(originPath, tmpData)
			Expect(err).To(BeNil())
		})
		It("should recover if origin path does not exist", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default")
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/tmp_default")
			tmpData := []byte("tmp")
			err = fileGenerator(tmpPath, tmpData)
			Expect(err).To(BeNil())
			err = store.recoverDir(tmpPath)
			Expect(err).To(BeNil())
			Expect(fs.IfExists(tmpPath)).To(BeFalse())
			err = fileChecker(originPath, tmpData)
			Expect(err).To(BeNil())
		})
		It("should return error if tmp path is not a dir", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default")
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/tmp_default")
			originData := []byte("origin")
			tmpData := []byte("tmp")
			err = fileGenerator(originPath, originData)
			Expect(err).To(BeNil())
			err = writeFileAt(tmpPath, tmpData)
			Expect(err).To(BeNil())
			err = store.recoverDir(tmpPath)
			Expect(err).NotTo(BeNil())
		})
		It("should return error if origin path is not a dir", func() {
			originPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/default")
			tmpPath := filepath.Join(baseDir, "kubelet/configmaps.v1.core/tmp_default")
			originData := []byte("origin")
			tmpData := []byte("tmp")
			err = writeFileAt(originPath, originData)
			Expect(err).To(BeNil())
			err = fileGenerator(tmpPath, tmpData)
			Expect(err).To(BeNil())
			err = store.recoverDir(tmpPath)
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Test Recover", func() {
		It("should recover cache", func() {
			tmpResourcesDir := filepath.Join(baseDir, "kubelet/tmp_configmaps")
			originResourcesDir := filepath.Join(baseDir, "kubelet/configmaps")
			tmpPodsFilePath := filepath.Join(baseDir, "kubelet/pods/default/tmp_coredns")
			originPodsFilePath := filepath.Join(baseDir, "kubelet/pods/default/coredns")
			err = fileGenerator(tmpResourcesDir, []byte("tmp_configmaps"))
			Expect(err).To(BeNil())
			err = writeFileAt(tmpPodsFilePath, []byte("tmp_pods"))
			Expect(err).To(BeNil())

			err = store.Recover()
			Expect(err).To(BeNil())
			err = fileChecker(originResourcesDir, []byte("tmp_configmaps"))
			Expect(err).To(BeNil())
			buf, err := checkFileAt(originPodsFilePath)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte("tmp_pods")))
		})
	})
})

var _ = Describe("Test DiskStorage Internal Functions", func() {
	// TODO:
})

var _ = Describe("Test DiskStorage Exposed Functions", func() {
	var store storage.Store
	var baseDir string
	var err error
	BeforeEach(func() {
		// We need to create a dir for each Context to avoid ErrStorageAccessConflict.
		baseDir = filepath.Join(diskStorageTestBaseDir, uuid.New().String())
		store, err = NewDiskStorage(baseDir)
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		err = os.RemoveAll(baseDir)
		Expect(err).To(BeNil())
	})

	// TODO: ErrUnrecognizedKey
	Context("Test Create", func() {
		var pod *v1.Pod
		var podKey storage.Key
		var podKeyInfo storage.KeyBuildInfo
		var podBytes []byte
		BeforeEach(func() {
			podKeyInfo = storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			}
			pod, podKey, err = generatePod(store.KeyFunc, &podObj, podKeyInfo)
			Expect(err).To(BeNil())
			podBytes, err = marshalObj(pod)
			Expect(err).To(BeNil())
		})
		It("should create key with content at local file system", func() {
			err = store.Create(podKey, podBytes)
			Expect(err).To(BeNil())

			By("ensure the file has been created")
			buf, err := checkFileAt(filepath.Join(baseDir, podKey.Key()))
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(podBytes))
		})
		It("should create the dir if it is rootKey", func() {
			rootKeyInfo := storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			}
			rootKey, err := store.KeyFunc(rootKeyInfo)
			Expect(err).To(BeNil())
			err = store.Create(rootKey, []byte{})
			Expect(err).To(BeNil())
			info, err := os.Stat(filepath.Join(baseDir, rootKey.Key()))
			Expect(err).To(BeNil())
			Expect(info.IsDir()).To(BeTrue())
		})
		It("should return ErrKeyHasNoContent if it is not rootKey and has no content", func() {
			err = store.Create(podKey, []byte{})
			Expect(err).To(Equal(storage.ErrKeyHasNoContent))
		})
		It("should return ErrKeyIsEmpty if key is empty", func() {
			err = store.Create(storageKey{}, podBytes)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrKeyExists if key exists", func() {
			err = writeFileAt(filepath.Join(baseDir, podKey.Key()), podBytes)
			Expect(err).To(BeNil())
			err = store.Create(podKey, podBytes)
			Expect(err).To(Equal(storage.ErrKeyExists))
		})
	})

	Context("Test Delete", func() {
		var podKey storage.Key
		var podKeyInfo storage.KeyBuildInfo
		BeforeEach(func() {
			podKeyInfo = storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			}
			_, podKey, err = generateObjFiles(baseDir, store.KeyFunc, &podObj, podKeyInfo)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			// nothing to do
			// all generated files will be deleted when deleting the base dir of diskStorage.
		})

		It("should delete file of key from file system", func() {
			err = store.Delete(podKey)
			Expect(err).To(BeNil())
			_, err = os.Stat(filepath.Join(baseDir, podKey.Key()))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
		It("should delete key with no error if it does not exist in file system", func() {
			_, newPodKey, err := generatePod(store.KeyFunc, &podObj, storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			err = store.Delete(newPodKey)
			Expect(err).To(BeNil())
		})
		It("should delete the dir if it is rootKey", func() {
			rootKey, err := store.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			})
			Expect(err).To(BeNil())
			err = store.Delete(rootKey)
			Expect(err).To(BeNil())
			_, err = os.Stat(filepath.Join(baseDir, rootKey.Key()))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
		It("should return ErrKeyIsEmpty if key is empty", func() {
			err = store.Delete(storageKey{})
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
	})

	Context("Test Get", func() {
		var podKey storage.Key
		var podBytes []byte
		BeforeEach(func() {
			podBytes, podKey, err = generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			// nothing to do
			// all generated files will be deleted when deleting the base dir of diskStorage.
		})

		It("should return the content of file of this key", func() {
			buf, err := store.Get(podKey)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(podBytes))
		})
		It("should return ErrKeyIsEmpty if key is empty", func() {
			_, err = store.Get(storageKey{})
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if key does not exist", func() {
			newPodKey, err := store.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			_, err = store.Get(newPodKey)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return ErrKeyHasNoContent if it is a root key", func() {
			rootKey, err := store.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			})
			Expect(err).To(BeNil())
			_, err = store.Get(rootKey)
			Expect(err).To(Equal(storage.ErrKeyHasNoContent))
		})
	})

	Context("Test List", func() {
		var podNamespace1Num, podNamespace2Num int
		var namespace1, namespace2 string
		var podNamespace1ObjBytes, podNamespace2ObjBytes map[storage.Key][]byte
		var rootKeyInfo storage.KeyBuildInfo
		var rootKey storage.Key
		BeforeEach(func() {
			podNamespace1Num, podNamespace2Num = 6, 4
			namespace1, namespace2 = "kube-system", "default"
			podNamespace1ObjBytes, podNamespace2ObjBytes = make(map[storage.Key][]byte, podNamespace1Num), make(map[storage.Key][]byte, podNamespace2Num)
			rootKeyInfo = storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Group:     "",
				Version:   "v1",
			}
			rootKey, err = store.KeyFunc(rootKeyInfo)
			Expect(err).To(BeNil())
			// prepare pod files under namespaces of kube-system and default
			for i := 0; i < podNamespace1Num; i++ {
				genPodBytes, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Namespace: namespace1,
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				podNamespace1ObjBytes[genKey] = genPodBytes
			}
			for i := 0; i < podNamespace2Num; i++ {
				genPodBytes, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Namespace: namespace2,
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				podNamespace2ObjBytes[genKey] = genPodBytes
			}
		})
		AfterEach(func() {
			// nothing to do
			// all generated files will be deleted when deleting the base dir of diskStorage.
		})

		It("should get a list of all resources according to rootKey", func() {
			objBytes, err := store.List(rootKey)
			Expect(err).To(BeNil())
			allBytes := map[storage.Key][]byte{}
			gotBytes := map[storage.Key][]byte{}
			for i := range objBytes {
				objKey, err := keyFromPodObjectBytes(store.KeyFunc, objBytes[i])
				Expect(err).To(BeNil())
				gotBytes[objKey] = objBytes[i]
			}
			for k, b := range podNamespace1ObjBytes {
				allBytes[k] = b
			}
			for k, b := range podNamespace2ObjBytes {
				allBytes[k] = b
			}
			Expect(gotBytes).To(Equal(allBytes))
		})
		It("should get a list of resources under the same namespace according to rooKey", func() {
			rootKeyInfo.Namespace = namespace1
			rootKey, err = store.KeyFunc(rootKeyInfo)
			Expect(err).To(BeNil())
			objBytes, err := store.List(rootKey)
			Expect(err).To(BeNil())
			gotBytes := map[storage.Key][]byte{}
			for i := range objBytes {
				objKey, err := keyFromPodObjectBytes(store.KeyFunc, objBytes[i])
				Expect(err).To(BeNil())
				gotBytes[objKey] = objBytes[i]
			}
			Expect(gotBytes).To(Equal(podNamespace1ObjBytes))
		})
		It("should return ErrKeyIsEmpty if key is empty", func() {
			_, err = store.List(storageKey{})
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if the rootKey does no exist", func() {
			rootKeyInfo.Resources = "services"
			rootKey, err = store.KeyFunc(rootKeyInfo)
			Expect(err).To(BeNil())
			_, err := store.List(rootKey)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return empty slice if the rootKey exists but no keys have it as prefix", func() {
			path := filepath.Join(baseDir, "kubelet/services.v1.core")
			err = os.MkdirAll(path, 0755)
			Expect(err).To(BeNil())
			rootKeyInfo.Resources = "services"
			rootKey, err = store.KeyFunc(rootKeyInfo)
			Expect(err).To(BeNil())
			gotBytes, err := store.List(rootKey)
			Expect(err).To(BeNil())
			Expect(len(gotBytes)).To(BeZero())
		})
		It("should return the object bytes if the key specifies the single object", func() {
			var key storage.Key
			for k := range podNamespace1ObjBytes {
				key = k
				break
			}
			b, err := store.List(key)
			Expect(err).To(BeNil())
			Expect(len(b)).To(Equal(1))
			Expect(b[0]).To(Equal(podNamespace1ObjBytes[key]))
		})
	})

	Context("Test Update", func() {
		var existingPodRvUint64, comingPodRvUint64 uint64
		var existingPod, comingPod *v1.Pod
		var podKey storage.Key
		var existingPodBytes, comingPodBytes []byte
		BeforeEach(func() {
			By("set existing pod")
			existingPodRvUint64, comingPodRvUint64 = 100, 200
			existingPod = podObj.DeepCopy()
			existingPod.Name = uuid.New().String()
			existingPod.ResourceVersion = fmt.Sprintf("%d", existingPodRvUint64)

			By("set coming pod")
			comingPod = podObj.DeepCopy()
			comingPod.Name = existingPod.Name
			comingPod.ResourceVersion = fmt.Sprintf("%d", comingPodRvUint64)

			By("ensure existing pod and coming pod have the same key but different contents")
			existingPodKey, err := keyFromPodObject(store.KeyFunc, existingPod)
			Expect(err).To(BeNil())
			comingPodKey, err := keyFromPodObject(store.KeyFunc, comingPod)
			Expect(err).To(BeNil())
			Expect(comingPodKey).To(Equal(existingPodKey))
			podKey = existingPodKey
			existingPodBytes, err = marshalObj(existingPod)
			Expect(err).To(BeNil())
			comingPodBytes, err = marshalObj(comingPod)
			Expect(err).To(BeNil())
			Expect(existingPodBytes).NotTo(Equal(comingPodBytes))

			By("prepare existing pod file")
			err = writeFileAt(filepath.Join(baseDir, existingPodKey.Key()), existingPodBytes)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			// nothing to do
			// all generated files will be deleted when deleting the base dir of diskStorage.
		})

		It("should update file of key if rv is fresher", func() {
			// update it with new pod bytes
			buf, err := store.Update(podKey, comingPodBytes, comingPodRvUint64)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(comingPodBytes))
		})
		It("should return ErrIsNotObjectKey if key is a root key", func() {
			rootKey, err := store.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
			})
			Expect(err).To(BeNil())
			_, err = store.Update(rootKey, comingPodBytes, comingPodRvUint64)
			Expect(err).To(Equal(storage.ErrIsNotObjectKey))
		})
		It("should return ErrKeyIsEmpty if key is empty", func() {
			_, err = store.Update(storageKey{}, comingPodBytes, comingPodRvUint64)
			Expect(err).To(Equal(storage.ErrKeyIsEmpty))
		})
		It("should return ErrStorageNotFound if key does not exist", func() {
			newPodKey, err := store.KeyFunc(storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			_, err = store.Update(newPodKey, []byte("data of non-existing pod"), existingPodRvUint64+1)
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return ErrUpdateConflict if rv is staler", func() {
			By("prepare a coming pod with older rv")
			comingPodRvUint64 = existingPodRvUint64 - 10
			comingPod.ResourceVersion = fmt.Sprintf("%d", comingPodRvUint64)
			comingPodBytes, err = marshalObj(comingPod)
			Expect(err).To(BeNil())
			Expect(comingPodBytes).NotTo(Equal(existingPodBytes))
			comingPodKey, err := keyFromPodObject(store.KeyFunc, comingPod)
			Expect(err).To(BeNil())
			Expect(comingPodKey).To(Equal(podKey))

			By("update with coming pod obj of old rv")
			buf, err := store.Update(podKey, comingPodBytes, comingPodRvUint64)
			Expect(err).To(Equal(storage.ErrUpdateConflict))
			Expect(buf).To(Equal(existingPodBytes))
		})
	})

	Context("Test ListResourceKeysOfComponent", func() {
		var podNamespace1Num, podNamespace2Num, nodeNum int
		var namespace1, namespace2 string
		var podNamespace1Keys, podNamespace2Keys map[storage.Key]struct{}
		var allPodKeys map[storage.Key]struct{}
		When("cache namespaced resource", func() {
			BeforeEach(func() {
				podNamespace1Num, podNamespace2Num = 2, 3
				namespace1, namespace2 = "kube-system", "default"
				podNamespace1Keys = make(map[storage.Key]struct{}, podNamespace1Num)
				podNamespace2Keys = make(map[storage.Key]struct{}, podNamespace2Num)
				allPodKeys = make(map[storage.Key]struct{})
				for i := 0; i < podNamespace1Num; i++ {
					_, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
						Component: "kubelet",
						Resources: "pods",
						Namespace: namespace1,
						Group:     "",
						Version:   "v1",
						Name:      uuid.New().String(),
					})
					Expect(err).To(BeNil())
					podNamespace1Keys[genKey] = struct{}{}
					allPodKeys[genKey] = struct{}{}
				}
				for i := 0; i < podNamespace2Num; i++ {
					_, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
						Component: "kubelet",
						Resources: "pods",
						Namespace: namespace2,
						Group:     "",
						Version:   "v1",
						Name:      uuid.New().String(),
					})
					Expect(err).To(BeNil())
					podNamespace2Keys[genKey] = struct{}{}
					allPodKeys[genKey] = struct{}{}
				}
			})
			AfterEach(func() {
				// nothing to do
				// all generated files will be deleted when deleting the base dir of diskStorage.
			})
			It("should get all keys of resource of component", func() {
				gotKeys, err := store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "pods",
				})
				Expect(err).To(BeNil())
				gotKeysMap := make(map[storage.Key]struct{})
				for _, k := range gotKeys {
					gotKeysMap[k] = struct{}{}
				}
				Expect(gotKeysMap).To(Equal(allPodKeys))
			})
			It("should return ErrStorageNotFound if the cache of component cannot be found or the resource has not been cached", func() {
				_, err = store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "services",
				})
				Expect(err).To(Equal(storage.ErrStorageNotFound))
				_, err = store.ListResourceKeysOfComponent("kube-proxy", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "pods",
				})
				Expect(err).To(Equal(storage.ErrStorageNotFound))
			})
		})
		When("cache non-namespaced resource", func() {
			var nodeKeys map[storage.Key]struct{}
			BeforeEach(func() {
				nodeNum = 20
				nodeKeys = make(map[storage.Key]struct{}, nodeNum)
				for i := 0; i < nodeNum; i++ {
					_, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &nodeObj, storage.KeyBuildInfo{
						Component: "kubelet",
						Resources: "nodes",
						Group:     "",
						Version:   "v1",
						Name:      uuid.New().String(),
					})
					Expect(err).To(BeNil())
					nodeKeys[genKey] = struct{}{}
				}
			})
			AfterEach(func() {
				// nothing to do
				// all generated files will be deleted when deleting the base dir of diskStorage.
			})
			It("should get all keys of gvr of component", func() {
				gotKeys, err := store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "nodes",
				})
				Expect(err).To(BeNil())
				for _, k := range gotKeys {
					_, ok := nodeKeys[k]
					Expect(ok).To(BeTrue())
					delete(nodeKeys, k)
				}
				Expect(len(nodeKeys)).To(BeZero())
			})
			It("should return ErrStorageNotFound if the cache of component cannot be found or the resource has not been cached", func() {
				_, err = store.ListResourceKeysOfComponent("kube-proxy", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "nodes",
				})
				Expect(err).To(Equal(storage.ErrStorageNotFound))
				_, err = store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "services",
				})
				Expect(err).To(Equal(storage.ErrStorageNotFound))
			})
		})
		It("should return ErrEmptyComponent if component is empty", func() {
			_, err = store.ListResourceKeysOfComponent("", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			})
			Expect(err).To(Equal(storage.ErrEmptyComponent))
		})
		It("should return ErrEmptyResource if gvr is empty", func() {
			_, err = store.ListResourceKeysOfComponent("kubelet", schema.GroupVersionResource{})
			Expect(err).To(Equal(storage.ErrEmptyResource))
		})
	})

	Context("Test ReplaceComponentList", func() {
		var podNamespace1Num, podNamespace2Num int
		var namespace1, namespace2 string
		var nodeNum int
		var contentsOfPodInNamespace1, contentsOfPodInNamespace2, contentsOfNode map[storage.Key][]byte
		BeforeEach(func() {
			namespace1, namespace2 = "default", "kube-system"
			podNamespace1Num, podNamespace2Num = 10, 20
			nodeNum = 5
			contentsOfPodInNamespace1 = make(map[storage.Key][]byte, podNamespace1Num)
			contentsOfPodInNamespace2 = make(map[storage.Key][]byte, podNamespace2Num)
			contentsOfNode = make(map[storage.Key][]byte, nodeNum)
			for i := 0; i < podNamespace1Num; i++ {
				genBytes, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Namespace: namespace1,
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				contentsOfPodInNamespace1[genKey] = genBytes
			}
			for i := 0; i < podNamespace2Num; i++ {
				genBytes, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &podObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "pods",
					Namespace: namespace2,
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				contentsOfPodInNamespace2[genKey] = genBytes
			}
			for i := 0; i < nodeNum; i++ {
				genBytes, genKey, err := generateObjFiles(baseDir, store.KeyFunc, &nodeObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "nodes",
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				contentsOfNode[genKey] = genBytes
			}
		})
		AfterEach(func() {
			// nothing to do
			// all generated files will be deleted when deleting the base dir of diskStorage.
		})

		It("should replace all cached non-namespaced objs of gvr of component", func() {
			newNodeNum := nodeNum + 2
			newNodeContents := make(map[storage.Key][]byte, newNodeNum)
			for i := 0; i < newNodeNum; i++ {
				genNode, genKey, err := generateNode(store.KeyFunc, &nodeObj, storage.KeyBuildInfo{
					Component: "kubelet",
					Resources: "nodes",
					Group:     "",
					Version:   "v1",
					Name:      uuid.New().String(),
				})
				Expect(err).To(BeNil())
				genBytes, err := marshalObj(genNode)
				Expect(err).To(BeNil())
				newNodeContents[genKey] = genBytes
			}
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "nodes",
			}, "", newNodeContents)
			Expect(err).To(BeNil())

			By("check if files under kubelet/nodes.v1.core are replaced with newNodeContents")
			gotContents, err := getFilesUnderDir(filepath.Join(baseDir, "kubelet", "nodes.v1.core"))
			Expect(err).To(BeNil())
			Expect(len(gotContents)).To(Equal(newNodeNum))
			for k, c := range newNodeContents {
				_, name := filepath.Split(k.Key())
				buf, ok := gotContents[name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("name %s", name))
				Expect(buf).To(Equal(c))
			}
		})

		When("replace namespaced objs", func() {
			var newPodNamespace string
			var newPodNum int
			var newPodContents map[storage.Key][]byte
			BeforeEach(func() {
				newPodNamespace = namespace1
				newPodNum = podNamespace1Num + 2
				newPodContents = make(map[storage.Key][]byte, newPodNum)
				By("generate new pod files to store")
				for i := 0; i < newPodNum; i++ {
					genPod, genKey, err := generatePod(store.KeyFunc, &podObj, storage.KeyBuildInfo{
						Component: "kubelet",
						Resources: "pods",
						Namespace: newPodNamespace,
						Group:     "",
						Version:   "v1",
						Name:      uuid.New().String(),
					})
					Expect(err).To(BeNil())
					genBytes, err := marshalObj(genPod)
					Expect(err).To(BeNil())
					newPodContents[genKey] = genBytes
				}
			})
			It("should replace cached objs of all namespaces of gvr of component if namespace is not provided", func() {
				allContents := make(map[storage.Key][]byte)
				for k, c := range newPodContents {
					allContents[k] = c
				}

				By("generate new pod files under another namespace to store")
				newPodNamespace2 := "new-namespace"
				newPodNamespace2Num := 2
				newPodNamespace2Contents := make(map[storage.Key][]byte)
				for i := 0; i < newPodNamespace2Num; i++ {
					genPod, genKey, err := generatePod(store.KeyFunc, &podObj, storage.KeyBuildInfo{
						Component: "kubelet",
						Resources: "pods",
						Group:     "",
						Version:   "v1",
						Namespace: newPodNamespace2,
						Name:      uuid.New().String(),
					})
					Expect(err).To(BeNil())
					genBytes, err := marshalObj(genPod)
					Expect(err).To(BeNil())
					allContents[genKey] = genBytes
					newPodNamespace2Contents[genKey] = genBytes
				}

				By("call ReplaceComponentList without provided namespace")
				err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "pods",
				}, "", allContents)
				Expect(err).To(BeNil())

				By("ensure files under newPodNamespace have been replaced")
				gotContents, err := getFilesUnderDir(filepath.Join(baseDir, "kubelet", "pods.v1.core", newPodNamespace))
				Expect(err).To(BeNil())
				Expect(len(gotContents)).To(Equal(newPodNum))
				for k, c := range newPodContents {
					_, name := filepath.Split(k.Key())
					buf, ok := gotContents[name]
					Expect(ok).To(BeTrue())
					Expect(buf).To(Equal(c))
				}

				By("ensure files under newPodNamespace2 have been created")
				gotContents, err = getFilesUnderDir(filepath.Join(baseDir, "kubelet", "pods.v1.core", newPodNamespace2))
				Expect(err).To(BeNil())
				Expect(len(gotContents)).To(Equal(newPodNamespace2Num))
				for k, c := range newPodNamespace2Contents {
					_, name := filepath.Split(k.Key())
					buf, ok := gotContents[name]
					Expect(ok).To(BeTrue())
					Expect(buf).To(Equal(c))
				}

				By("ensure files under other namespaces have been removed")
				entries, err := os.ReadDir(filepath.Join(baseDir, "kubelet", "pods.v1.core"))
				Expect(err).To(BeNil())
				Expect(len(entries)).To(Equal(2))
				Expect(entries[0].IsDir() && entries[1].IsDir())
				Expect((entries[0].Name() == newPodNamespace && entries[1].Name() == newPodNamespace2) ||
					(entries[0].Name() == newPodNamespace2 && entries[1].Name() == newPodNamespace)).To(BeTrue())
			})
			It("should replace cached objs under the namespace of gvr of component if namespace is provided", func() {
				By("call ReplaceComponentList")
				err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "pods",
				}, newPodNamespace, newPodContents)
				Expect(err).To(BeNil())

				By("ensure files under the specified namespace have been replaced")
				gotContents, err := getFilesUnderDir(filepath.Join(baseDir, "kubelet", "pods.v1.core", newPodNamespace))
				Expect(err).To(BeNil())
				Expect(len(gotContents)).To(Equal(newPodNum))
				for k, c := range newPodContents {
					_, name := filepath.Split(k.Key())
					buf, ok := gotContents[name]
					Expect(ok).To(BeTrue())
					Expect(buf).To(Equal(c))
				}

				By("ensure pod files of namespace2 are unchanged")
				curContents, err := getFilesUnderDir(filepath.Join(baseDir, "kubelet", "pods.v1.core", namespace2))
				Expect(err).To(BeNil())
				Expect(len(curContents)).To(Equal(podNamespace2Num))
				for k, c := range contentsOfPodInNamespace2 {
					_, name := filepath.Split(k.Key())
					buf, ok := curContents[name]
					Expect(ok).To(BeTrue())
					Expect(buf).To(Equal(c))
				}
			})
		})

		It("should return error if namespace is provided but the gvr is non-namespaced", func() {
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			}, "default", contentsOfNode)
			Expect(err).Should(HaveOccurred())
		})
		It("should create base dirs and files if this kind of gvr has never been cached", func() {
			By("generate a new pod obj in non-existing namespace")
			newPod, newPodKey, err := generatePod(store.KeyFunc, &podObj, storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "pods",
				Namespace: "nonexisting",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			newPodBytes, err := marshalObj(newPod)
			Expect(err).To(BeNil())

			By("call ReplaceComponentList")
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			}, "nonexisting", map[storage.Key][]byte{
				newPodKey: newPodBytes,
			})
			Expect(err).To(BeNil())

			By("check if the new pod file and its dir have been created")
			buf, err := checkFileAt(filepath.Join(baseDir, newPodKey.Key()))
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(newPodBytes))
		})
		It("should create base dirs and files if the component has no resource of gvr cached", func() {
			By("generate a new pod obj cached by new component")
			newPod, newPodKey, err := generatePod(store.KeyFunc, &podObj, storage.KeyBuildInfo{
				Component: "kube-proxy",
				Resources: "pods",
				Namespace: "default",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			newPodBytes, err := marshalObj(newPod)
			Expect(err).To(BeNil())

			By("call ReplaceComponentList")
			err = store.ReplaceComponentList("kube-proxy", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			}, "default", map[storage.Key][]byte{
				newPodKey: newPodBytes,
			})
			Expect(err).To(BeNil())

			By("check if the new pod file and its dir have been created")
			buf, err := checkFileAt(filepath.Join(baseDir, newPodKey.Key()))
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(newPodBytes))
		})
		It("should create the base dir when contents is empty", func() {
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
				Group:    "storage.k8s.io",
				Version:  "v1",
				Resource: "csidrivers",
			}, "", nil)
			Expect(err).To(BeNil())
			entries, err := os.ReadDir(filepath.Join(baseDir, "kubelet", "csidrivers.v1.storage.k8s.io"))
			Expect(err).To(BeNil(), fmt.Sprintf("failed to read dir %v", err))
			Expect(len(entries)).To(BeZero())
		})
		It("should return ErrEmptyComponent if component is empty", func() {
			err = store.ReplaceComponentList("", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			}, "default", map[storage.Key][]byte{})
			Expect(err).To(Equal(storage.ErrEmptyComponent))
		})
		It("should return ErrEmptyResource if gvr is empty", func() {
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{}, "default", map[storage.Key][]byte{})
			Expect(err).To(Equal(storage.ErrEmptyResource))
		})
		It("should return ErrInvalidContent if some contents are not the specified gvr", func() {
			err = store.ReplaceComponentList("kubelet", schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "nodes",
			}, "", contentsOfPodInNamespace1)
			Expect(err).To(Equal(storage.ErrInvalidContent))
		})
	})

	Context("Test DeleteComponentResources", func() {
		It("should delete all files of component", func() {
			_, _, err = generateObjFiles(baseDir, store.KeyFunc, &nodeObj, storage.KeyBuildInfo{
				Component: "kubelet",
				Resources: "nodes",
				Group:     "",
				Version:   "v1",
				Name:      uuid.New().String(),
			})
			Expect(err).To(BeNil())
			err = store.DeleteComponentResources("kubelet")
			Expect(err).To(BeNil())
			_, err = os.Stat(filepath.Join(baseDir, "kubelet"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
		It("should return ErrEmptyComponent if component is empty", func() {
			err = store.DeleteComponentResources("")
			Expect(err).To(Equal(storage.ErrEmptyComponent))
		})
	})

	Context("Test SaveClusterInfo", func() {
		It("should create new version content if it does not exists", func() {
			err = store.SaveClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Version,
				UrlPath:         "/version",
			}, []byte(versionJSONBytes))
			Expect(err).To(BeNil())
			buf, err := checkFileAt(filepath.Join(baseDir, string(storage.Version)))
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte(versionJSONBytes)))
		})
		It("should overwrite existing version content in storage", func() {
			newVersionBytes := []byte("new bytes")
			path := filepath.Join(baseDir, string(storage.Version))
			err = writeFileAt(path, []byte(versionJSONBytes))
			Expect(err).To(BeNil())
			err = store.SaveClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Version,
				UrlPath:         "/version",
			}, newVersionBytes)
			Expect(err).To(BeNil())
			buf, err := checkFileAt(path)
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte(newVersionBytes)))
		})
		It("should return ErrUnknownClusterInfoType if it is unknown ClusterInfoType", func() {
			err = store.SaveClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Unknown,
			}, nil)
			Expect(err).To(Equal(storage.ErrUnknownClusterInfoType))
		})
		// TODO: add unit-test for api-versions and api-resources
	})

	Context("Test GetClusterInfo", func() {
		It("should get version info", func() {
			path := filepath.Join(baseDir, string(storage.Version))
			err = writeFileAt(path, []byte(versionJSONBytes))
			Expect(err).To(BeNil())
			buf, err := store.GetClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Version,
				UrlPath:         "/version",
			})
			Expect(err).To(BeNil())
			Expect(buf).To(Equal([]byte(versionJSONBytes)))
		})
		It("should return ErrStorageNotFound if version info has not been cached", func() {
			_, err = store.GetClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Version,
				UrlPath:         "/version",
			})
			Expect(err).To(Equal(storage.ErrStorageNotFound))
		})
		It("should return ErrUnknownClusterInfoType if it is unknown ClusterInfoType", func() {
			_, err = store.GetClusterInfo(storage.ClusterInfoKey{
				ClusterInfoType: storage.Unknown,
			})
			Expect(err).To(Equal(storage.ErrUnknownClusterInfoType))
		})
		// TODO: add unit-test for api-versions and api-resources
	})
})

func checkFileAt(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeFileAt(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dir at %s, %v", dir, path)
	}

	return os.WriteFile(path, content, 0766)
}

func keyFromPodObject(keyFunc func(storage.KeyBuildInfo) (storage.Key, error), pod *v1.Pod) (storage.Key, error) {
	ns, name := pod.Namespace, pod.Name
	keyInfo := storage.KeyBuildInfo{
		Component: "kubelet",
		Resources: "pods",
		Namespace: ns,
		Group:     "",
		Version:   "v1",
		Name:      name,
	}
	return keyFunc(keyInfo)
}

func keyFromPodObjectBytes(keyFunc func(storage.KeyBuildInfo) (storage.Key, error), objBytes []byte) (storage.Key, error) {
	serializer := jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, jsonserializer.SerializerOptions{})
	pod := &v1.Pod{}
	_, _, err := serializer.Decode(objBytes, nil, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to deserializer obj, %v", err)
	}
	return keyFromPodObject(keyFunc, pod)
}

func marshalObj(obj runtime.Object) ([]byte, error) {
	return json.Marshal(obj)
}

func generatePod(keyFunc func(storage.KeyBuildInfo) (storage.Key, error), template *v1.Pod, keyInfo storage.KeyBuildInfo) (*v1.Pod, storage.Key, error) {
	genKey, err := keyFunc(keyInfo)
	if err != nil {
		return nil, nil, err
	}
	copy := template.DeepCopy()
	copy.Name = keyInfo.Name
	copy.Namespace = keyInfo.Namespace
	return copy, genKey, err
}

func generateNode(keyFunc func(storage.KeyBuildInfo) (storage.Key, error), template *v1.Node, keyInfo storage.KeyBuildInfo) (*v1.Node, storage.Key, error) {
	genKey, err := keyFunc(keyInfo)
	if err != nil {
		return nil, nil, err
	}
	copy := template.DeepCopy()
	copy.Name = keyInfo.Name
	return copy, genKey, err
}

func generateObjFiles(baseDir string, keyFunc func(storage.KeyBuildInfo) (storage.Key, error), template runtime.Object, keyInfo storage.KeyBuildInfo) ([]byte, storage.Key, error) {
	var genObj runtime.Object
	var genKey storage.Key
	var err error

	switch obj := template.(type) {
	case *v1.Pod:
		genObj, genKey, err = generatePod(keyFunc, obj, keyInfo)
	case *v1.Node:
		genObj, genKey, err = generateNode(keyFunc, obj, keyInfo)
	default:
		return nil, nil, fmt.Errorf("unrecognized object type: %v", obj)
	}
	if err != nil {
		return nil, nil, err
	}

	jsonBytes, err := marshalObj(genObj)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal obj, %v", err)
	}
	err = writeFileAt(filepath.Join(baseDir, genKey.Key()), jsonBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write to file, %v", err)
	}
	return jsonBytes, genKey, nil
}

func getFilesUnderDir(dir string) (map[string][]byte, error) {
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	contents := map[string][]byte{}
	for i := range infos {
		if infos[i].Type().IsRegular() {
			buf, err := os.ReadFile(filepath.Join(dir, infos[i].Name()))
			if err != nil {
				return nil, err
			}
			contents[infos[i].Name()] = buf
		}
	}
	return contents, nil
}

func TestDiskStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiskStorage Suite")
}

func TestExtractInfoFromPath(t *testing.T) {
	cases := map[string]struct {
		baseDir    string
		path       string
		isRoot     bool
		want       []string
		wantErrOut string
	}{
		"normal case": {
			baseDir:    "/tmp/baseDir",
			path:       "/tmp/baseDir/kubelet/pods.v1.core/default/podname-a",
			isRoot:     false,
			want:       []string{"kubelet", "pods.v1.core", "default", "podname-a"},
			wantErrOut: "",
		},
		"root path": {
			baseDir:    "/tmp/baseDir",
			path:       "/tmp/baseDir/kubelet/pods.v1.core/default",
			isRoot:     true,
			want:       []string{"kubelet", "pods.v1.core", "default", ""},
			wantErrOut: "",
		},
		"few elements in path": {
			baseDir:    "/tmp/baseDir",
			path:       "/tmp/baseDir",
			isRoot:     true,
			want:       []string{"", "", "", ""},
			wantErrOut: "",
		},
		"too many elements of path": {
			baseDir:    "/tmp/baseDir",
			path:       "/tmp/baseDir/kubelet/kubelet/pods.v1.core/default/podname-a",
			isRoot:     false,
			want:       []string{"", "", "", ""},
			wantErrOut: "invalid path /tmp/baseDir/kubelet/kubelet/pods.v1.core/default/podname-a",
		},
		"path does not under the baseDir": {
			baseDir:    "/tmp/baseDir",
			path:       "/other/baseDir/kubelet/pods.v1.core/default/podname-a",
			isRoot:     false,
			want:       []string{"", "", "", ""},
			wantErrOut: "path /other/baseDir/kubelet/pods.v1.core/default/podname-a does not under /tmp/baseDir",
		},
	}

	for c, d := range cases {
		t.Run(c, func(t *testing.T) {
			comp, res, ns, n, err := extractInfoFromPath(d.baseDir, d.path, d.isRoot)
			var gotErrOut string
			if err != nil {
				gotErrOut = err.Error()
			}
			if d.wantErrOut != gotErrOut {
				t.Errorf("failed at case: %s, wrong error, want: %s, got: %s", c, d.wantErrOut, gotErrOut)
			}
			got := strings.Join([]string{comp, res, ns, n}, " ")
			want := strings.Join(d.want, " ")
			if got != want {
				t.Errorf("failed at case: %s, want: %s, got: %s", c, want, got)
			}
		})
	}
}

func TestIfEnhancement(t *testing.T) {
	cases := []struct {
		existingFile map[string][]byte
		want         bool
		description  string
	}{
		{
			existingFile: map[string][]byte{
				"/kubelet/pods/default/nginx": []byte("nginx-pod"),
			},
			want:        false,
			description: "should not run in enhancement mode if there's old cache",
		},
		{
			existingFile: map[string][]byte{},
			want:         true,
			description:  "should run in enhancement mode if there's no old cache",
		},
		{
			existingFile: map[string][]byte{
				"/kubelet/pods.v1.core/default/nginx": []byte("nginx-pod"),
			},
			want:        true,
			description: "should run in enhancement mode if all cache are resource.version.group format",
		},
		{
			existingFile: map[string][]byte{
				"/kubelet/pods.v1.core/default/nginx":             []byte("nginx-pod"),
				"/_internal/restmapper/cache-crd-restmapper.conf": []byte("restmapper"),
				"/version": []byte("version"),
			},
			want:        true,
			description: "should ignore internal dirs",
		},
	}

	for _, c := range cases {
		baseDir := diskStorageTestBaseDir
		t.Run(c.description, func(t *testing.T) {
			os.RemoveAll(baseDir)
			fsOperator := fs.FileSystemOperator{}
			fsOperator.CreateDir(baseDir)

			for f, b := range c.existingFile {
				path := filepath.Join(baseDir, f)
				if err := fsOperator.CreateFile(path, b); err != nil {
					t.Errorf("failed to create file %s, %v", path, err)
				}
			}

			mode, err := ifEnhancement(baseDir, fsOperator)
			if err != nil {
				t.Errorf("failed to create disk storage, %v", err)
			}
			if mode != c.want {
				t.Errorf("unexpected running mode, want: %v, got: %v", c.want, mode)
			}
		})
	}
}
