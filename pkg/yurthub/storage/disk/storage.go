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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/utils"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

const (
	CacheBaseDir = "/etc/kubernetes/cache/"
	StorageName  = "local-disk"
	tmpPrefix    = "tmp_"
)

// TODO: lock should block, and also add comment about it
// TODO: should optimize the efficiency of the lock mechanism
type diskStorage struct {
	sync.Mutex
	baseDir          string
	keyPendingStatus map[string]struct{}
	serializer       runtime.Serializer
	// listSelectorCollector map[string]string
	fsOperator *fs.FileSystemOperator
}

// NewDiskStorage creates a storage.Store for caching data into local disk
func NewDiskStorage(dir string) (storage.Store, error) {
	if dir == "" {
		klog.Infof("disk cache path is empty, set it by default %s", CacheBaseDir)
		dir = CacheBaseDir
	}

	fsOperator := &fs.FileSystemOperator{}

	if err := fsOperator.CreateDir(dir); err != nil && err != fs.ErrExists {
		return nil, fmt.Errorf("failed to create cache path %s, %v", dir, err)
	}

	// prune suffix "/" of dir
	dir = strings.TrimSuffix(dir, "/")

	ds := &diskStorage{
		keyPendingStatus: make(map[string]struct{}),
		baseDir:          dir,
		serializer:       json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{}),
		fsOperator:       fsOperator,
	}

	err := ds.Recover()
	if err != nil {
		// we should ensure that there no tmp file last when local storage start to work.
		// Otherwise, it means the baseDir cannot serve as local storage dir, because there're some subpath
		// cannot manipulated by this local storage, which will have bad influence when the local storage is working.
		// So, we'd better return error to avoid unknown problems.
		return nil, fmt.Errorf("could not recover local storage, %v, and skip the error", err)
	}
	return ds, nil
}

// Name will return the name of this storage
func (ds *diskStorage) Name() string {
	return StorageName
}

// Create will create a new file with content. key indicates the path of the file.
func (ds *diskStorage) Create(key storage.Key, content []byte) error {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return err
	}
	if !ds.lockKey(key) {
		return storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(key)

	path := filepath.Join(ds.baseDir, key.Key())
	if key.IsRootKey() {
		// If it is rootKey, create the dir for it. Refer to #258.
		return ds.fsOperator.CreateDir(path)
	}
	err := ds.fsOperator.CreateFile(path, content)
	if err == fs.ErrExists {
		return storage.ErrKeyExists
	}
	if err != nil {
		return fmt.Errorf("failed to create file %s, %v", path, err)
	}
	return nil
}

// Delete will delete the file that specified by key.
func (ds *diskStorage) Delete(key storage.Key) error {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return err
	}

	if !ds.lockKey(key) {
		return storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(key)

	path := filepath.Join(ds.baseDir, key.Key())
	// TODO: do we need to delete root key
	if key.IsRootKey() {
		return ds.fsOperator.DeleteDir(path)
	}
	if err := ds.fsOperator.DeleteFile(path); err != nil {
		return fmt.Errorf("failed to delete file %s, %v", path, err)
	}

	return nil
}

// Get will get content from the regular file that specified by key.
// If key points to a dir, return ErrKeyHasNoContent.
func (ds *diskStorage) Get(key storage.Key) ([]byte, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return []byte{}, storage.ErrKeyIsEmpty
	}

	if !ds.lockKey(key) {
		return nil, storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(key)

	path := filepath.Join(ds.baseDir, key.Key())
	buf, err := ds.fsOperator.Read(path)
	switch err {
	case nil:
		return buf, nil
	case fs.ErrNotExists:
		return nil, storage.ErrStorageNotFound
	case fs.ErrIsNotFile:
		return nil, storage.ErrKeyHasNoContent
	default:
		return buf, fmt.Errorf("failed to read file at %s, %v", path, err)
	}
}

// List will get contents of all files recursively under the root dir pointed by the rootKey.
// If the root dir of this rootKey does not exist, return ErrStorageNotFound.
func (ds *diskStorage) List(key storage.Key) ([][]byte, error) {
	if err := utils.ValidateKey(key, storageKey{}); err != nil {
		return [][]byte{}, err
	}

	if !ds.lockKey(key) {
		return nil, storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(key)

	if !key.IsRootKey() {
		return nil, storage.ErrIsNotRootKey
	}

	bb := make([][]byte, 0)
	absPath := filepath.Join(ds.baseDir, key.Key())
	files, err := ds.fsOperator.List(absPath, fs.ListModeFiles, true)
	if err == fs.ErrNotExists {
		return nil, storage.ErrStorageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get all files under %s, %v", absPath, err)
	}
	for _, filePath := range files {
		buf, err := ds.fsOperator.Read(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file at %s, %v", filePath, err)
		}
		bb = append(bb, buf)
	}
	return bb, nil
}

// Update will update the file pointed by the key. It will check the rv of
// stored obj and update it only when the rv in argument is fresher than what is stored.
// It will return the content that finally stored in the file pointed by key.
// Update works in a backup way, which means it will first backup the original file, and then
// write the content into it.
func (ds *diskStorage) Update(key storage.Key, content []byte, rv uint64) ([]byte, error) {
	if err := utils.ValidateKV(key, content, storageKey{}); err != nil {
		return nil, err
	}

	if key.IsRootKey() {
		return nil, storage.ErrIsNotObjectKey
	}

	if !ds.lockKey(key) {
		return nil, storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(key)

	absPath := filepath.Join(ds.baseDir, key.Key())
	old, err := ds.fsOperator.Read(absPath)
	if err == fs.ErrNotExists {
		return nil, storage.ErrStorageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s, %v", absPath, err)
	}

	klog.V(4).Infof("find key %s exists when updating it", key.Key())
	ok, err := ds.ifFresherThan(old, rv)
	if err != nil {
		return nil, fmt.Errorf("failed to get rv of file %s, %v", absPath, err)
	}
	if !ok {
		return old, storage.ErrUpdateConflict
	}

	// update the file
	tmpPath := filepath.Join(ds.baseDir, getTmpKey(key).Key())
	if err := ds.fsOperator.Rename(absPath, tmpPath); err != nil {
		return nil, fmt.Errorf("failed to backup file %s, %v", absPath, err)
	}
	if err := ds.fsOperator.CreateFile(absPath, content); err != nil {
		// We can ensure that the file actually exists, so it should not be ErrNotExists
		return nil, fmt.Errorf("failed to write to file %s, %v", absPath, err)
	}
	if err := ds.fsOperator.DeleteFile(tmpPath); err != nil {
		return nil, fmt.Errorf("failed to delete backup file %s, %v", tmpPath, err)
	}
	return content, nil
}

// ListResourceKeysOfComponent will get all names of files recursively under the dir
// of the resource belonging to the component.
func (ds *diskStorage) ListResourceKeysOfComponent(component string, resource string) ([]storage.Key, error) {
	rootKey, err := ds.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Resources: resource,
	})
	if err != nil {
		return nil, err
	}

	if !ds.lockKey(rootKey) {
		return nil, storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(rootKey)

	absPath := filepath.Join(ds.baseDir, rootKey.Key())
	files, err := ds.fsOperator.List(absPath, fs.ListModeFiles, true)
	if err == fs.ErrNotExists {
		return nil, storage.ErrStorageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list files at %s, %v", filepath.Join(ds.baseDir, rootKey.Key()), err)
	}

	keys := make([]storage.Key, len(files))
	for i, filePath := range files {
		_, _, ns, n, err := extractInfoFromPath(ds.baseDir, filePath, false)
		if err != nil {
			klog.Errorf("failed when list keys of resource %s of component %s, %v", component, resource, err)
			continue
		}
		// We can ensure that component and resource can't be empty
		// so ignore the err.
		key, _ := ds.KeyFunc(storage.KeyBuildInfo{
			Component: component,
			Resources: resource,
			Namespace: ns,
			Name:      n,
		})
		keys[i] = key
	}
	return keys, nil
}

// ReplaceComponentList will replace the component list in a back-up way.
// It will first backup the original dir as tmpdir, including all its subdirs, and then clear the
// original dir and write contents into it. If the yurthub break down and restart, interrupting the previous
// ReplaceComponentList, the diskStorage will recover the data with backup in the tmpdir.
func (ds *diskStorage) ReplaceComponentList(component string, resource string, namespace string, contents map[storage.Key][]byte) error {
	rootKey, err := ds.KeyFunc(storage.KeyBuildInfo{
		Component: component,
		Resources: resource,
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

	// if ok, err := ds.canReplace(rootKey, selector); !ok {
	// 	return fmt.Errorf("cannot replace %s, %v", rootKey.Key(), err)
	// }

	if !ds.lockKey(rootKey) {
		return storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(rootKey)

	// 1. mv old dir into tmp_dir when rootKey dir already exists
	absPath := filepath.Join(ds.baseDir, rootKey.Key())
	tmpRootKey := getTmpKey(rootKey)
	tmpPath := filepath.Join(ds.baseDir, tmpRootKey.Key())
	if !fs.IfExists(absPath) {
		if err := ds.fsOperator.CreateDir(absPath); err != nil {
			return fmt.Errorf("failed to create dir at %s", absPath)
		}
	}
	if ok, err := fs.IsDir(absPath); err == nil && !ok {
		return fmt.Errorf("%s is not a dir", absPath)
	} else if err != nil {
		return fmt.Errorf("failed to check the path %s, %v", absPath, err)
	}
	// absPath exists and is a dir
	if err := ds.fsOperator.Rename(absPath, tmpPath); err != nil {
		return err
	}

	// 2. create new file with contents
	// TODO: if error happens, we may need retry mechanism, or add some mechanism to do consistency check.
	for key, data := range contents {
		path := filepath.Join(ds.baseDir, key.Key())
		if err := ds.fsOperator.CreateDir(filepath.Dir(path)); err != nil && err != fs.ErrExists {
			klog.Errorf("failed to create dir at %s, %v", filepath.Dir(path), err)
			continue
		}
		if err := ds.fsOperator.CreateFile(path, data); err != nil {
			klog.Errorf("failed to write data to %s, %v", path, err)
			continue
		}
		klog.V(4).Infof("[diskStorage] ReplaceComponentList store data at %s", path)
	}

	//  3. delete old tmp dir
	return ds.fsOperator.DeleteDir(tmpPath)
}

// DeleteComponentResources will delete all resources cached for component.
func (ds *diskStorage) DeleteComponentResources(component string) error {
	if component == "" {
		return storage.ErrEmptyComponent
	}
	rootKey := storageKey{
		path:      component,
		isRootKey: true,
	}
	if !ds.lockKey(rootKey) {
		return storage.ErrStorageAccessConflict
	}
	defer ds.unLockKey(rootKey)

	absKey := filepath.Join(ds.baseDir, rootKey.Key())
	if err := ds.fsOperator.DeleteDir(absKey); err != nil {
		return fmt.Errorf("failed to delete path %s, %v", absKey, err)
	}
	return nil
}

// Recover will walk the baseDir of this diskStorage, and try to recover the storage
// using backup file. It works when yurthub or the node breaks down and restart.
//
// Note:
// If a dir/file is a tmp dir/file, then we assume that any parent path should not be tmp path.
// Because we lock the path when manipulating it.
func (ds *diskStorage) Recover() error {
	recoveredDir := map[string]struct{}{}
	err := filepath.Walk(ds.baseDir, func(path string, info os.FileInfo, err error) error {
		for p := range recoveredDir {
			if strings.HasPrefix(path, p) {
				return nil
			}
		}

		if err != nil {
			return err
		}

		if isTmpFile(path) {
			switch {
			case info.Mode().IsDir():
				if err := ds.recoverDir(path); err != nil {
					return fmt.Errorf("failed to recover dir %s, %v", path, err)
				}
				recoveredDir[path] = struct{}{}
			case info.Mode().IsRegular():
				if err := ds.recoverFile(path); err != nil {
					return fmt.Errorf("failed to recover file %s, %v", path, err)
				}
			default:
				klog.Warningf("unrecognized file %s when recovering diskStorage", path)
			}
		}

		return nil
	})

	return err
}

func (ds *diskStorage) recoverFile(tmpPath string) error {
	if ok, err := fs.IsRegularFile(tmpPath); err != nil || !ok {
		return fmt.Errorf("failed at tmp path %s, isRegularFile: %v, error: %v", tmpPath, ok, err)
	}

	tmpKey := strings.TrimPrefix(tmpPath, ds.baseDir)
	key := getKey(tmpKey)
	path := filepath.Join(ds.baseDir, key)
	if fs.IfExists(path) {
		if ok, err := fs.IsRegularFile(path); err != nil || !ok {
			return fmt.Errorf("failed at origin path %s, isRegularFile: %v, error: %v", path, ok, err)
		}
		if err := ds.fsOperator.DeleteFile(path); err != nil {
			return fmt.Errorf("failed to delete file at %s, %v", path, err)
		}
	}
	if err := ds.fsOperator.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func (ds *diskStorage) recoverDir(tmpPath string) error {
	if ok, err := fs.IsDir(tmpPath); err != nil || !ok {
		return fmt.Errorf("failed at tmp path %s, isDir: %v, error: %v", tmpPath, ok, err)
	}

	tmpKey := strings.TrimPrefix(tmpPath, ds.baseDir)
	key := getKey(tmpKey)
	path := filepath.Join(ds.baseDir, key)
	if fs.IfExists(path) {
		if ok, err := fs.IsDir(path); err != nil || !ok {
			return fmt.Errorf("failed at origin path %s, isDir: %v, error: %v", path, ok, err)
		}
		if err := ds.fsOperator.DeleteDir(path); err != nil {
			return fmt.Errorf("failed to delete dir at %s, %v", path, err)
		}
	}
	if err := ds.fsOperator.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func (ds *diskStorage) lockKey(key storage.Key) bool {
	keyStr := key.Key()
	ds.Lock()
	defer ds.Unlock()
	if _, ok := ds.keyPendingStatus[keyStr]; ok {
		klog.Infof("key(%s) storage is pending, just skip it", keyStr)
		return false
	}

	for pendingKey := range ds.keyPendingStatus {
		if len(keyStr) > len(pendingKey) {
			if strings.Contains(keyStr, fmt.Sprintf("%s/", pendingKey)) {
				klog.Infof("key(%s) storage is pending, skip to store key(%s)", pendingKey, keyStr)
				return false
			}
		} else {
			if strings.Contains(pendingKey, fmt.Sprintf("%s/", keyStr)) {
				klog.Infof("key(%s) storage is pending, skip to store key(%s)", pendingKey, keyStr)
				return false
			}
		}
	}
	ds.keyPendingStatus[keyStr] = struct{}{}
	return true
}

// TODO: move to cache manager
// func (ds *diskStorage) canReplace(key storage.Key, selector string) (bool, error) {
// 	keyStr := key.Key()
// 	ds.Lock()
// 	defer ds.Unlock()
// 	if oldSelector, ok := ds.listSelectorCollector[keyStr]; ok {
// 		if oldSelector != selector {
// 			// list requests that have the same path but with different selector, for example:
// 			// request1: http://{ip:port}/api/v1/default/pods?labelSelector=foo=bar
// 			// request2: http://{ip:port}/api/v1/default/pods?labelSelector=foo2=bar2
// 			// because func queryListObject() will get all pods for both requests instead of
// 			// getting pods by request selector. so cache manager can not support same path list
// 			// requests that has different selector.
// 			klog.Warningf("list requests that have the same path but with different selector, skip cache for %s", keyStr)
// 			return false, fmt.Errorf("selector conflict, old selector is %s, current selector is %s", oldSelector, selector)
// 		}
// 	} else {
// 		// list requests that get the same resources but with different path, for example:
// 		// request1: http://{ip/port}/api/v1/pods?fieldSelector=spec.nodeName=foo
// 		// request2: http://{ip/port}/api/v1/default/pods?fieldSelector=spec.nodeName=foo
// 		// because func queryListObject() will get all pods for both requests instead of
// 		// getting pods by request selector. so cache manager can not support getting same resource
// 		// list requests that has different path.
// 		for k := range ds.listSelectorCollector {
// 			if (len(k) > len(keyStr) && strings.Contains(k, keyStr)) || (len(k) < len(keyStr) && strings.Contains(keyStr, k)) {
// 				klog.Warningf("list requests that get the same resources but with different path, skip cache for %s", key)
// 				return false, fmt.Errorf("path conflict, old path is %s, current path is %s", k, keyStr)
// 			}
// 		}
// 		ds.listSelectorCollector[keyStr] = selector
// 	}
// 	return true, nil
// }

func (ds *diskStorage) ifFresherThan(oldObj []byte, newRV uint64) (bool, error) {
	// check resource version
	unstructuredObj := &unstructured.Unstructured{}
	curObj, _, err := ds.serializer.Decode(oldObj, nil, unstructuredObj)
	if err != nil {
		return false, fmt.Errorf("failed to decode obj, %v", err)
	}
	curRv, err := ObjectResourceVersion(curObj)
	if err != nil {
		return false, fmt.Errorf("failed to get rv of obj, %v", err)
	}
	if newRV < curRv {
		return false, nil
	}
	return true, nil
}

func (ds *diskStorage) unLockKey(key storage.Key) {
	ds.Lock()
	defer ds.Unlock()
	delete(ds.keyPendingStatus, key.Key())
}

func getTmpKey(key storage.Key) storageKey {
	dir, file := filepath.Split(key.Key())
	return storageKey{
		path:      filepath.Join(dir, fmt.Sprintf("%s%s", tmpPrefix, file)),
		isRootKey: key.IsRootKey(),
	}
}

func isTmpFile(path string) bool {
	_, file := filepath.Split(path)
	return strings.HasPrefix(file, tmpPrefix)
}

func getKey(tmpKey string) string {
	dir, file := filepath.Split(tmpKey)
	return filepath.Join(dir, strings.TrimPrefix(file, tmpPrefix))
}

func extractInfoFromPath(baseDir, path string, isRoot bool) (component, resource, namespace, name string, err error) {
	if !strings.HasPrefix(path, baseDir) {
		err = fmt.Errorf("path %s does not under %s", path, baseDir)
		return
	}
	trimedPath := strings.TrimPrefix(path, baseDir)
	trimedPath = strings.TrimPrefix(trimedPath, "/")
	elems := strings.Split(trimedPath, "/")
	if len(elems) > 4 {
		err = fmt.Errorf("invalid path %s", path)
		return
	}
	switch len(elems) {
	case 0:
	case 1:
		component = elems[0]
	case 2:
		component, resource = elems[0], elems[1]
	case 3:
		component, resource = elems[0], elems[1]
		if isRoot {
			namespace = elems[2]
		} else {
			name = elems[2]
		}
	case 4:
		component, resource, namespace, name = elems[0], elems[1], elems[2], elems[3]
	}
	return
}

func ObjectResourceVersion(obj runtime.Object) (uint64, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	version := accessor.GetResourceVersion()
	if len(version) == 0 {
		return 0, nil
	}
	return strconv.ParseUint(version, 10, 64)
}
