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
	"strings"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
	coordinatorconstants "github.com/openyurtio/openyurt/pkg/yurthub/yurtcoordinator/constants"
)

type storageKeySet map[storageKey]struct{}

// Difference will return keys in s but not in s2
func (s storageKeySet) Difference(s2 storageKeySet) storageKeySet {
	keys := storageKeySet{}
	if s2 == nil {
		for k := range s {
			keys[k] = struct{}{}
		}
		return keys
	}

	for k := range s {
		if _, ok := s2[k]; !ok {
			keys[k] = struct{}{}
		}
	}

	return keys
}

type keyCache struct {
	m map[schema.GroupVersionResource]storageKeySet
}

// Do not directly modify value returned from functions of componentKeyCache, such as Load.
// Because it usually returns reference of internal objects for efficiency.
// The format in file is:
// component0#group.version.resource:key0,key1;group.version.resource:key2,key3...
// component1#group.version.resource:key4,key5...
// ...
type componentKeyCache struct {
	sync.Mutex
	ctx context.Context
	// map component to keyCache
	cache                     map[string]keyCache
	filePath                  string
	keyFunc                   func(storage.KeyBuildInfo) (storage.Key, error)
	fsOperator                fs.FileSystemOperator
	etcdClient                *clientv3.Client
	poolScopedResourcesGetter func() []schema.GroupVersionResource
}

func (c *componentKeyCache) Recover() error {
	var buf []byte
	var err error
	if buf, err = c.fsOperator.Read(c.filePath); err == fs.ErrNotExists {
		if err := c.fsOperator.CreateFile(c.filePath, []byte{}); err != nil {
			return fmt.Errorf("could not create cache file at %s, %v", c.filePath, err)
		}
	} else if err != nil {
		return fmt.Errorf("could not recover key cache from %s, %v", c.filePath, err)
	}

	if len(buf) != 0 {
		// We've got content from file
		cache, err := unmarshal(buf)
		if err != nil {
			return fmt.Errorf("could not parse file content at %s, %v", c.filePath, err)
		}
		c.cache = cache
	}

	poolScopedKeyset, err := c.getPoolScopedKeyset()
	if err != nil {
		return fmt.Errorf("could not get pool-scoped keys, %v", err)
	}
	// Overwrite the data we recovered from local disk, if any. Because we
	// only respect to the resources stored in yurt-coordinator to recover the
	// pool-scoped keys.
	c.cache[coordinatorconstants.DefaultPoolScopedUserAgent] = *poolScopedKeyset

	return nil
}

func (c *componentKeyCache) getPoolScopedKeyset() (*keyCache, error) {
	keys := &keyCache{m: make(map[schema.GroupVersionResource]storageKeySet)}
	getFunc := func(key string) (*clientv3.GetResponse, error) {
		getCtx, cancel := context.WithTimeout(c.ctx, defaultTimeout)
		defer cancel()
		return c.etcdClient.Get(getCtx, key, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	}
	for _, gvr := range c.poolScopedResourcesGetter() {
		rootKey, err := c.keyFunc(storage.KeyBuildInfo{
			Component: coordinatorconstants.DefaultPoolScopedUserAgent,
			Group:     gvr.Group,
			Version:   gvr.Version,
			Resources: gvr.Resource,
		})
		if err != nil {
			return nil, fmt.Errorf("could not generate keys for %s, %v", gvr.String(), err)
		}
		getResp, err := getFunc(rootKey.Key())
		if err != nil {
			return nil, fmt.Errorf("could not get from etcd for %s, %v", gvr.String(), err)
		}

		for _, kv := range getResp.Kvs {
			ns, name, err := getNamespaceAndNameFromKeyPath(string(kv.Key))
			if err != nil {
				return nil, fmt.Errorf("could not parse namespace and name of %s", kv.Key)
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
				return nil, fmt.Errorf("could not create resource key for %v", kv.Key)
			}

			if _, ok := keys.m[gvr]; !ok {
				keys.m[gvr] = storageKeySet{key.(storageKey): {}}
			} else {
				keys.m[gvr][key.(storageKey)] = struct{}{}
			}
		}
	}
	return keys, nil
}

// Load returns keyCache of component which contains keys of all gvr.
func (c *componentKeyCache) Load(component string) (keyCache, bool) {
	c.Lock()
	defer c.Unlock()
	cache, ok := c.cache[component]
	return cache, ok
}

// AddKey will add key to the key cache of such component. If the component
// does not have its cache, it will be created first.
func (c *componentKeyCache) AddKey(component string, key storageKey) {
	c.Lock()
	defer c.Unlock()
	defer c.flush()
	if _, ok := c.cache[component]; !ok {
		c.cache[component] = keyCache{m: map[schema.GroupVersionResource]storageKeySet{
			key.gvr: {
				key: struct{}{},
			},
		}}
		return
	}

	keyCache := c.cache[component]
	if keyCache.m == nil {
		keyCache.m = map[schema.GroupVersionResource]storageKeySet{
			key.gvr: {
				key: struct{}{},
			},
		}
		return
	}

	if _, ok := keyCache.m[key.gvr]; !ok {
		keyCache.m[key.gvr] = storageKeySet{key: {}}
		return
	}
	keyCache.m[key.gvr][key] = struct{}{}
}

// DeleteKey deletes specified key from the key cache of the component.
func (c *componentKeyCache) DeleteKey(component string, key storageKey) {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.cache[component]; !ok {
		return
	}
	if c.cache[component].m == nil {
		return
	}
	if _, ok := c.cache[component].m[key.gvr]; !ok {
		return
	}
	delete(c.cache[component].m[key.gvr], key)
	c.flush()
}

// LoadOrStore will load the keyset of specified gvr from cache of the component if it exists,
// otherwise it will be created with passed-in keyset argument. It will return the key set
// finally in the component cache, and a bool value indicating whether the returned key set
// is loaded or stored.
func (c *componentKeyCache) LoadOrStore(component string, gvr schema.GroupVersionResource, keyset storageKeySet) (storageKeySet, bool) {
	c.Lock()
	defer c.Unlock()
	if cache, ok := c.cache[component]; ok {
		if cache.m == nil {
			cache.m = make(map[schema.GroupVersionResource]storageKeySet)
		}

		if set, ok := cache.m[gvr]; ok {
			return set, true
		} else {
			cache.m[gvr] = keyset
			c.flush()
			return keyset, false
		}
	} else {
		c.cache[component] = keyCache{
			m: map[schema.GroupVersionResource]storageKeySet{
				gvr: keyset,
			},
		}
		c.flush()
		return keyset, false
	}
}

// LoadAndDelete will load and delete the key cache of specified component.
// Return the original cache and true if it was deleted, otherwise empty cache and false.
func (c *componentKeyCache) LoadAndDelete(component string) (keyCache, bool) {
	c.Lock()
	defer c.Unlock()
	if cache, ok := c.cache[component]; ok {
		delete(c.cache, component)
		c.flush()
		return cache, true
	}
	return keyCache{}, false
}

func (c *componentKeyCache) flush() error {
	buf := marshal(c.cache)
	if err := c.fsOperator.Write(c.filePath, buf); err != nil {
		return fmt.Errorf("could not flush cache to file %s, %v", c.filePath, err)
	}
	return nil
}

func marshal(cache map[string]keyCache) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	for comp, ks := range cache {
		line := bytes.NewBufferString(fmt.Sprintf("%s#", comp))
		for gvr, s := range ks.m {
			gvrStr := strings.Join([]string{gvr.Group, gvr.Version, gvr.Resource}, "_")
			keys := make([]string, 0, len(s))
			for k := range s {
				keys = append(keys, k.Key())
			}
			line.WriteString(fmt.Sprintf("%s:%s;", gvrStr, strings.Join(keys, ",")))
		}
		if len(ks.m) != 0 {
			// discard last ';'
			line.Truncate(line.Len() - 1)
		}
		line.WriteByte('\n')
		buf.Write(line.Bytes())
	}
	if buf.Len() != 0 {
		// discard last '\n'
		buf.Truncate(buf.Len() - 1)
	}
	return buf.Bytes()
}

func unmarshal(buf []byte) (map[string]keyCache, error) {
	cache := map[string]keyCache{}
	if len(buf) == 0 {
		return cache, nil
	}

	lines := strings.Split(string(buf), "\n")
	for i, l := range lines {
		s := strings.Split(l, "#")
		if len(s) != 2 {
			return nil, fmt.Errorf("could not parse line %d, invalid format", i)
		}
		comp := s[0]

		keySet := keyCache{m: map[schema.GroupVersionResource]storageKeySet{}}
		if len(s[1]) > 0 {
			gvrKeys := strings.Split(s[1], ";")
			for _, gvrKey := range gvrKeys {
				ss := strings.Split(gvrKey, ":")
				if len(ss) != 2 {
					return nil, fmt.Errorf("could not parse gvr keys %s at line %d, invalid format", gvrKey, i)
				}
				gvrStrs := strings.Split(ss[0], "_")
				if len(gvrStrs) != 3 {
					return nil, fmt.Errorf("could not parse gvr %s at line %d, invalid format", ss[0], i)
				}
				gvr := schema.GroupVersionResource{
					Group:    gvrStrs[0],
					Version:  gvrStrs[1],
					Resource: gvrStrs[2],
				}

				set := storageKeySet{}
				if len(ss[1]) != 0 {
					keys := strings.Split(ss[1], ",")
					for _, k := range keys {
						key := storageKey{
							comp: comp,
							path: k,
							gvr:  gvr,
						}
						set[key] = struct{}{}
					}
				}
				keySet.m[gvr] = set
			}
		}
		cache[comp] = keySet
	}
	return cache, nil
}

// We assume that path points to a namespaced resource.
func getNamespaceAndNameFromKeyPath(path string) (string, string, error) {
	elems := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(elems) < 2 {
		return "", "", fmt.Errorf("unrecognized path: %s", path)
	}

	return elems[len(elems)-2], elems[len(elems)-1], nil
}
