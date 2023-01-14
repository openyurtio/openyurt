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
	"bytes"
	"context"
	"fmt"
	"github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/resources"
	"strings"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"

	coordinatorconstants "github.com/openyurtio/openyurt/pkg/yurthub/poolcoordinator/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

// type state int

// const (
// 	done       state = 0
// 	processing state = 1
// )

// type status struct

type keySet struct {
	m map[storageKey]struct{}
}

// Difference will return keys in s but not in s2
func (s keySet) Difference(s2 keySet) []storageKey {
	keys := []storageKey{}
	if s2.m == nil {
		for k := range s.m {
			keys = append(keys, k)
		}
		return keys
	}

	for k := range s.m {
		if _, ok := s2.m[k]; !ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// Do not directly modify value returned from functions of componentKeyCache, such as Load.
// Because it usually returns reference of internal objects for efficiency.
// The format in file is:
// component0:key0,key1...
// component1:key0,key1...
// ...
type componentKeyCache struct {
	sync.Mutex
	ctx        context.Context
	cache      map[string]keySet
	filePath   string
	keyFunc    func(storage.KeyBuildInfo) (storage.Key, error)
	fsOperator fs.FileSystemOperator
	etcdClient *clientv3.Client
}

func (c *componentKeyCache) Recover() error {
	var buf []byte
	var err error
	if buf, err = c.fsOperator.Read(c.filePath); err == fs.ErrNotExists {
		if err := c.fsOperator.CreateFile(c.filePath, []byte{}); err != nil {
			return fmt.Errorf("failed to create cache file at %s, %v", c.filePath, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to recover key cache from %s, %v", c.filePath, err)
	}

	if len(buf) != 0 {
		// We've got content from file
		lines := strings.Split(string(buf), "\n")
		for i, l := range lines {
			s := strings.Split(l, ":")
			if len(s) != 2 {
				return fmt.Errorf("failed to parse line %d, invalid format", i)
			}
			comp, keys := s[0], strings.Split(s[1], ",")
			ks := keySet{m: map[storageKey]struct{}{}}
			for _, key := range keys {
				ks.m[storageKey{
					comp: comp,
					path: key,
				}] = struct{}{}
			}
			c.cache[comp] = ks
		}
	}

	poolScopedKeyset, err := c.getPoolScopedKeyset()
	if err != nil {
		return fmt.Errorf("failed to get pool-scoped keys, %v", err)
	}
	// Overwrite the data we recovered from local disk, if any. Because we
	// only respect to the resources stored in pool-coordinator to recover the
	// pool-scoped keys.
	c.cache[coordinatorconstants.DefaultPoolScopedUserAgent] = *poolScopedKeyset

	return nil
}

func (c *componentKeyCache) getPoolScopedKeyset() (*keySet, error) {
	keys := &keySet{m: map[storageKey]struct{}{}}
	for _, gvr := range resources.GetPoolScopeResources() {
		getCtx, cancel := context.WithTimeout(c.ctx, defaultTimeout)
		defer cancel()
		rootKey, err := c.keyFunc(storage.KeyBuildInfo{
			Component: coordinatorconstants.DefaultPoolScopedUserAgent,
			Group:     gvr.Group,
			Version:   gvr.Version,
			Resources: gvr.Resource,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate keys for %s, %v", gvr.String(), err)
		}
		getResp, err := c.etcdClient.Get(getCtx, rootKey.Key(), clientv3.WithPrefix(), clientv3.WithKeysOnly())
		if err != nil {
			return nil, fmt.Errorf("failed to get from etcd for %s, %v", gvr.String(), err)
		}

		for _, kv := range getResp.Kvs {
			ns, name, err := getNamespaceAndNameFromKeyPath(string(kv.Key))
			if err != nil {
				return nil, fmt.Errorf("failed to parse namespace and name of %s", kv.Key)
			}
			key, err := c.keyFunc(storage.KeyBuildInfo{
				Component: coordinatorconstants.DefaultPoolScopedUserAgent,
				Group:     gvr.Group,
				Version:   gvr.Version,
				Resources: gvr.Resource,
				Namespace: ns,
				Name:      name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create resource key for %v", kv.Key)
			}
			keys.m[key.(storageKey)] = struct{}{}
		}
	}
	return keys, nil
}

func (c *componentKeyCache) Load(component string) (keySet, bool) {
	c.Lock()
	defer c.Unlock()
	cache, ok := c.cache[component]
	return cache, ok
}

func (c *componentKeyCache) AddKey(component string, key storageKey) {
	c.Lock()
	defer c.Unlock()
	defer c.flush()
	if _, ok := c.cache[component]; !ok {
		c.cache[component] = keySet{m: map[storageKey]struct{}{
			key: {},
		}}
		return
	}

	keyset := c.cache[component]
	if keyset.m == nil {
		keyset.m = map[storageKey]struct{}{
			key: {},
		}
		return
	}

	c.cache[component].m[key] = struct{}{}
}

func (c *componentKeyCache) DeleteKey(component string, key storageKey) {
	c.Lock()
	defer c.Unlock()
	delete(c.cache[component].m, key)
	c.flush()
}

func (c *componentKeyCache) LoadOrStore(component string, keyset keySet) (keySet, bool) {
	c.Lock()
	defer c.Unlock()
	if cache, ok := c.cache[component]; ok {
		return cache, true
	} else {
		c.cache[component] = keyset
		c.flush()
		return keyset, false
	}
}

func (c *componentKeyCache) LoadAndDelete(component string) (keySet, bool) {
	c.Lock()
	defer c.Unlock()
	if cache, ok := c.cache[component]; ok {
		delete(c.cache, component)
		c.flush()
		return cache, true
	}
	return keySet{}, false
}

func (c *componentKeyCache) DeleteAllKeysOfComponent(component string) {
	c.Lock()
	defer c.Unlock()
	delete(c.cache, component)
	c.flush()
}

// func (c *componentKeyCache) MarkAsProcessing() {

// }

// func (c *componentKeyCache) MarkAsDone() {

// }

func (c *componentKeyCache) flush() error {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	for comp, ks := range c.cache {
		line := bytes.NewBufferString(fmt.Sprintf("%s:", comp))
		keys := make([]string, 0, len(ks.m))
		for k := range ks.m {
			keys = append(keys, k.Key())
		}
		line.WriteString(strings.Join(keys, ","))
		line.WriteByte('\n')
		buf.Write(line.Bytes())
	}
	if buf.Len() != 0 {
		// discard last '\n'
		buf.Truncate(buf.Len() - 1)
	}
	if err := c.fsOperator.Write(c.filePath, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to flush cache to file %s, %v", c.filePath, err)
	}
	return nil
}

// We assume that path points to a namespaced resource.
func getNamespaceAndNameFromKeyPath(path string) (string, string, error) {
	elems := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(elems) < 2 {
		return "", "", fmt.Errorf("unrecognized path: %s", path)
	}

	return elems[len(elems)-2], elems[len(elems)-1], nil
}
