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

func TestIPMAnager(t *testing.T) {
	ipRanges := "192.168.0.1-192.168.1.5, 10.0.0.1-10.0.0.3"
	manager, err := vip.NewIPManager(ipRanges)
	if err != nil {
		t.Fatalf("Failed to create IPManager: %v", err)
	}

	t.Run("get ip", func(t *testing.T) {
		// Test getting IPVRID
		ipVRID, err := manager.Get()
		if err != nil {
			t.Errorf("Failed to get IPVRID: %v", err)
		}
		if ipVRID.IPs == nil || len(ipVRID.IPs) == 0 {
			t.Error("IP is empty")
		}
		if ipVRID.VRID < 0 || ipVRID.VRID >= vip.VRIDMAXVALUE {
			t.Error("Invalid VRID")
		}
	})

	t.Run("release ip", func(t *testing.T) {
		// Test releasing IPVRID
		ipVRID, _ := manager.Get()
		err = manager.Release(ipVRID)
		if err != nil {
			t.Errorf("Failed to release IPVRID: %v", err)
		}
	})

	t.Run("get ip when none are available", func(t *testing.T) {
		// Test getting IPVRID when none are available
		m, err := vip.NewIPManager("192.168.0.1")
		if err != nil {
			t.Errorf("Failed to create IPManager: %v", err)
		}

		_, err = m.Get()
		if err != nil {
			t.Errorf("Expected not error but get error: %v", err)
		}

		_, err = m.Get()
		if err == nil {
			t.Error("Expected error when no IPVRID is available")
		}
	})

	t.Run("release ip that is not in use", func(t *testing.T) {
		// Test releasing IPVRID that is not in use
		ipVRID := vip.IPVRID{IPs: []string{"10.0.0.1"}, VRID: 0}
		err = manager.Release(ipVRID)
		if err == nil {
			t.Errorf("Expected error: %v when releasing unused IPVRID", err)
		}
	})

	t.Run("release ip that is in use", func(t *testing.T) {
		// Test releasing IPVRID that is in use
		ipVRID, _ := manager.Get()
		err = manager.Release(ipVRID)
		if err != nil {
			t.Errorf("Failed to release IPVRID: %v", err)
		}
	})

	t.Run("sync ip with repeat", func(t *testing.T) {
		// Test syncing IPVRIDs
		ipVRIDs := []vip.IPVRID{
			{IPs: []string{"192.168.0.1"}, VRID: 0},
			{IPs: []string{"192.168.0.2"}, VRID: 1},
			{IPs: []string{"10.0.0.1"}, VRID: 0},
			{IPs: []string{"10.0.0.2"}, VRID: 1},
		}
		err = manager.Sync(ipVRIDs)
		if err != nil {
			t.Errorf("Failed to sync IPVRIDs: %v", err)
		}
	})

	t.Run("sync ip", func(t *testing.T) {
		// Test syncing IPVRIDs
		ipVRIDs := []vip.IPVRID{
			{IPs: []string{"192.168.0.1"}, VRID: 0},
			{IPs: []string{"192.168.0.2"}, VRID: 1},
			{IPs: []string{"10.0.0.1"}, VRID: 0},
			{IPs: []string{"10.0.0.2"}, VRID: 1},
		}
		err = manager.Sync(ipVRIDs)
		if err != nil {
			t.Errorf("Failed to sync IPVRIDs: %v", err)
		}
	})

	t.Run("sync ip with invalid vrid", func(t *testing.T) {
		// Test syncing IPVRIDs with invalid VRID
		ipVRIDs := []vip.IPVRID{
			{IPs: []string{"192.168.0.3"}, VRID: -1},
			{IPs: []string{"192.168.0.4"}, VRID: vip.VRIDMAXVALUE},
		}
		err = manager.Sync(ipVRIDs)
		if err == nil {
			t.Error("Expected error when syncing IPVRIDs with invalid VRID")
		}
	})

	t.Run("sync ip with ip not found", func(t *testing.T) {
		// Test syncing IPVRIDs with IP not found
		ipVRIDs := []vip.IPVRID{
			{IPs: []string{"192.168.2.1"}, VRID: 0},
		}
		err = manager.Sync(ipVRIDs)
		if err == nil {
			t.Error("Expected error when syncing IPVRIDs with IP not found")
		}
	})

}
