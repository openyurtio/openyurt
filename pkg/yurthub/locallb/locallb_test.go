/*
Copyright 2024 The OpenYurt Authors.

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

package locallb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockIptablesManager is a mock implementation of IptablesManagerInterface.
// It records whether methods were called and with what parameters, allowing for assertions in tests.
type mockIptablesManager struct {
	updateCalled bool
	cleanCalled  bool
	lastService  string
	lastAddrs    []string
	updateErr    error // Used to simulate an error from updateIptablesRules
	cleanErr     error // Used to simulate an error from cleanIptablesRules
}

func (m *mockIptablesManager) updateIptablesRules(tenantKasService string, apiserverAddrs []string) error {
	m.updateCalled = true
	m.lastService = tenantKasService
	m.lastAddrs = apiserverAddrs
	return m.updateErr
}

func (m *mockIptablesManager) cleanIptablesRules() error {
	m.cleanCalled = true
	return m.cleanErr
}

// newTestEndpoints is a helper function to quickly create Endpoints objects for testing.
func newTestEndpoints(name string, ips []string, port int32) *corev1.Endpoints {
	var addresses []corev1.EndpointAddress
	for _, ip := range ips {
		addresses = append(addresses, corev1.EndpointAddress{IP: ip})
	}
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: addresses,
				Ports: []corev1.EndpointPort{
					{Port: port},
				},
			},
		},
	}
}

func TestAddEndpoints(t *testing.T) {
	const serviceAddr = "10.0.0.1:6443"

	t.Run("Successfully add Endpoints", func(t *testing.T) {
		mockIPT := &mockIptablesManager{}
		manager := newLocalLBManagerWithDeps(serviceAddr, mockIPT)
		endpoints := newTestEndpoints("kube-apiserver", []string{"192.168.1.2", "192.168.1.3"}, 6443)
		manager.addEndpoints(endpoints)
		assert.True(t, mockIPT.updateCalled, "updateIptablesRules should have been called")
		assert.Equal(t, serviceAddr, mockIPT.lastService, "The service address passed was incorrect")
		assert.ElementsMatch(t, []string{"192.168.1.2:6443", "192.168.1.3:6443"}, manager.apiserverAddrs, "The manager's internal apiserver address list is incorrect")
		assert.ElementsMatch(t, manager.apiserverAddrs, mockIPT.lastAddrs, "The address list passed to updateIptablesRules was incorrect")
	})

	t.Run("Pass an object that is not an Endpoints type", func(t *testing.T) {
		mockIPT := &mockIptablesManager{}
		manager := newLocalLBManagerWithDeps(serviceAddr, mockIPT)
		manager.addEndpoints("not an endpoint")
		assert.False(t, mockIPT.updateCalled, "updateIptablesRules should not be called when the type is wrong")
	})
}

func TestUpdateEndpoints(t *testing.T) {
	const serviceAddr = "10.0.0.1:6443"

	t.Run("iptables should be updated when address list changes", func(t *testing.T) {
		mockIPT := &mockIptablesManager{}
		manager := newLocalLBManagerWithDeps(serviceAddr, mockIPT)
		manager.apiserverAddrs = []string{"192.168.1.2:6443", "192.168.1.3:6443"} // Initial state

		oldEndpoints := newTestEndpoints("kube-apiserver", []string{"192.168.1.2", "192.168.1.3"}, 6443)
		newEndpoints := newTestEndpoints("kube-apiserver", []string{"192.168.1.4", "192.168.1.5"}, 6443) // New addresses

		manager.updateEndpoints(oldEndpoints, newEndpoints)

		assert.True(t, mockIPT.updateCalled, "updateIptablesRules should be called when addresses change")
		// Old addresses are deleted, new addresses are added
		assert.ElementsMatch(t, []string{"192.168.1.4:6443", "192.168.1.5:6443"}, manager.apiserverAddrs)
		assert.ElementsMatch(t, manager.apiserverAddrs, mockIPT.lastAddrs)
	})

	t.Run("iptables should not be updated when address list is unchanged", func(t *testing.T) {
		mockIPT := &mockIptablesManager{}
		manager := newLocalLBManagerWithDeps(serviceAddr, mockIPT)

		oldEndpoints := newTestEndpoints("kube-apiserver", []string{"192.168.1.2", "192.168.1.3"}, 6443)
		newEndpoints := newTestEndpoints("kube-apiserver", []string{"192.168.1.3", "192.168.1.2"}, 6443) // Same content, different order

		manager.updateEndpoints(oldEndpoints, newEndpoints)

		assert.False(t, mockIPT.updateCalled, "updateIptablesRules should not be called when addresses are the same")
	})
}

func TestDeleteOldApiserverAddrs(t *testing.T) {
	manager := &locallbManager{}

	testCases := []struct {
		name         string
		initialAddrs []string
		toRemove     []string
		expected     []string
	}{
		{"Remove some elements from the list", []string{"a", "b", "c", "d"}, []string{"b", "d"}, []string{"a", "c"}},
		{"Remove all elements", []string{"a", "b"}, []string{"a", "b"}, []string{}},
		{"Remove non-existent elements", []string{"a", "b"}, []string{"c"}, []string{"a", "b"}},
		{"Remove from an empty list", []string{}, []string{"a"}, []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy to test on, to avoid modifying the original slice
			addrs := make([]string, len(tc.initialAddrs))
			copy(addrs, tc.initialAddrs)

			manager.deleteOldApiserverAddrs(&addrs, tc.toRemove)
			assert.ElementsMatch(t, tc.expected, addrs)
		})
	}
}

func TestCleanIptables(t *testing.T) {
	t.Run("Successfully clean", func(t *testing.T) {
		mockIPT := &mockIptablesManager{}
		manager := newLocalLBManagerWithDeps("", mockIPT)

		err := manager.CleanIptables()
		require.NoError(t, err)
		assert.True(t, mockIPT.cleanCalled, "cleanIptablesRules should have been called")
	})

	t.Run("Error occurs during cleaning", func(t *testing.T) {
		expectedErr := fmt.Errorf("failed to lock iptables")
		mockIPT := &mockIptablesManager{cleanErr: expectedErr}
		manager := newLocalLBManagerWithDeps("", mockIPT)

		err := manager.CleanIptables()
		require.Error(t, err)
		assert.Equal(t, expectedErr, err, "Should return the underlying error")
	})
}
