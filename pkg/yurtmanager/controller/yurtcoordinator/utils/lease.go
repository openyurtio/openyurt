/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package utils

import (
	"sync"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/constant"
)

type LeaseDelegatedCounter struct {
	v    map[string]int
	lock sync.Mutex
}

func NewLeaseDelegatedCounter() *LeaseDelegatedCounter {
	return &LeaseDelegatedCounter{
		v: make(map[string]int),
	}
}

func (dc *LeaseDelegatedCounter) Inc(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	if dc.v[name] >= constant.LeaseDelegationThreshold {
		return
	}
	dc.v[name] += 1
}

func (dc *LeaseDelegatedCounter) Dec(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	if dc.v[name] > 0 {
		dc.v[name] -= 1
	}
}

func (dc *LeaseDelegatedCounter) Reset(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	dc.v[name] = 0
}

func (dc *LeaseDelegatedCounter) Del(name string) {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	delete(dc.v, name)
}

func (dc *LeaseDelegatedCounter) Counter(name string) int {
	dc.lock.Lock()
	defer dc.lock.Unlock()

	return dc.v[name]
}
