package etcd

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

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
	cache      map[string]keySet
	filePath   string
	fsOperator fs.FileSystemOperator
}

func (c *componentKeyCache) Recover() error {
	var buf []byte
	var err error
	if buf, err = c.fsOperator.Read(c.filePath); err == fs.ErrNotExists {
		if err := c.fsOperator.CreateFile(c.filePath, []byte{}); err != nil {
			return fmt.Errorf("failed to create cache file at %s, %v", c.filePath, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to recover key cache from %s, %v", c.filePath, err)
	}

	// successfully read from file
	if len(buf) == 0 {
		return nil
	}
	lines := strings.Split(string(buf), "\n")
	for i, l := range lines {
		s := strings.Split(l, ":")
		if len(s) != 2 {
			return fmt.Errorf("failed to parse line %d, invalid format", i)
		}
		comp, keys := s[0], strings.Split(s[1], ",")
		ks := keySet{m: map[storageKey]struct{}{}}
		for _, key := range keys {
			ks.m[storageKey{path: key}] = struct{}{}
		}
		c.cache[comp] = ks
	}
	return nil
}

func (c *componentKeyCache) Load(component string) (keySet, bool) {
	c.Lock()
	defer c.Unlock()
	cache, ok := c.cache[component]
	return cache, ok
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

func newComponentKeyCache(filePath string) *componentKeyCache {
	return &componentKeyCache{
		filePath:   filePath,
		cache:      map[string]keySet{},
		fsOperator: fs.FileSystemOperator{},
	}
}
