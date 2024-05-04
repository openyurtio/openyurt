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

package viploadbalancer_test

import (
	"testing"

	vip "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

var (
	nodepool = "np123"
)

func TestVRIDManage(t *testing.T) {
	vm := vip.NewVRIDManager()

	t.Run("get vrid", func(t *testing.T) {
		// Test getting VRID
		vrid := vm.GetVRID(nodepool)
		if vrid != 0 {
			t.Errorf("Expected VRID 0, but got %d", vrid)
		}
	})

	t.Run("release vrid", func(t *testing.T) {
		// Test releasing VRID
		target := vm.GetVRID(nodepool)
		vm.ReleaseVRID(nodepool, target)
		vrid := vm.GetVRID(nodepool)
		if vrid != target {
			t.Errorf("Expected VRID %d after release, but got %d", target, vrid)
		}
	})

	t.Run("vrid is valid", func(t *testing.T) {
		// Test isValid function
		isValid := vm.IsValid(nodepool, 0)
		if !isValid {
			t.Errorf("Expected VRID 0 to be valid, but it's not")
		}
	})

	t.Run("vrid is not valid", func(t *testing.T) {
		// Test isValid function
		isValid := vm.IsValid(nodepool, -1)
		if isValid {
			t.Errorf("Expected VRID -1 to be invalid, but it's not")
		}
	})

	t.Run("sync vrid", func(t *testing.T) {
		// Test SyncVRID function
		vm.SyncVRID(nodepool, []int{1, 2, 3})
		target := 4
		vrid := vm.GetVRID(nodepool)
		if vrid != target {
			t.Errorf("Expected VRID %d after sync, but got %d", target, vrid)
		}
	})
}
