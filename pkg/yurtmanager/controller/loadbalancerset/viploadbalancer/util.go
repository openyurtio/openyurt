/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package viploadbalancer

import "sync"

const (
	MINVRIDLIMIT = 0
	MAXVRIDLIMIT = 255
	EVICTED      = -1
)

type VRIDManager struct {
	vridMap map[string]map[int]bool
	maxVRID int
	mutex   sync.Mutex
}

func NewVRIDManager() *VRIDManager {
	return &VRIDManager{
		vridMap: make(map[string]map[int]bool),
		maxVRID: MAXVRIDLIMIT,
	}
}

func (vm *VRIDManager) GetVRID(key string) int {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	if _, ok := vm.vridMap[key]; !ok {
		vm.vridMap[key] = make(map[int]bool)
	}

	for vrid := 0; vrid < vm.maxVRID; vrid++ {
		if !vm.vridMap[key][vrid] {
			vm.vridMap[key][vrid] = true
			return vrid
		}
	}

	return EVICTED
}

func (vm *VRIDManager) ReleaseVRID(key string, vrid int) {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	if _, ok := vm.vridMap[key]; !ok {
		return
	}

	delete(vm.vridMap[key], vrid)
}

func (vm *VRIDManager) IsValid(key string, vrid int) bool {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	if vrid < 0 || vrid > vm.maxVRID {
		return false
	}

	if _, ok := vm.vridMap[key][vrid]; !ok {
		return false
	}

	return true
}

func (vm *VRIDManager) SyncVRID(key string, vird []int) {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	if _, ok := vm.vridMap[key]; !ok {
		vm.vridMap[key] = make(map[int]bool)
	}

	for _, v := range vird {
		vm.vridMap[key][v] = true
	}
}
