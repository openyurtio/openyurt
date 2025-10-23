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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIPTables is a mock implementation of the IPTablesInterface for unit testing.
// It simulates iptables behavior in memory, such as creating chains and rules.
type mockIPTables struct {
	// Chains stores the mocked iptables rules.
	// The key is in the format "table/chain", and the value is the list of rules in that chain.
	Chains map[string][]string
}

// newMockIPTables creates an initialized mockIPTables.
func newMockIPTables() *mockIPTables {
	return &mockIPTables{
		Chains: make(map[string][]string),
	}
}

// --- mockIPTables implements all methods of the IPTablesInterface ---

func (m *mockIPTables) ChainExists(table, chain string) (bool, error) {
	key := fmt.Sprintf("%s/%s", table, chain)
	_, exists := m.Chains[key]
	return exists, nil
}

func (m *mockIPTables) NewChain(table, chain string) error {
	key := fmt.Sprintf("%s/%s", table, chain)
	if _, exists := m.Chains[key]; exists {
		return fmt.Errorf("chain `%s` already exists in table `%s`", chain, table)
	}
	// The real iptables List command includes the chain definition, so we simulate that here.
	m.Chains[key] = []string{fmt.Sprintf("-N %s", chain)}
	return nil
}

func (m *mockIPTables) Append(table, chain string, rulespec ...string) error {
	key := fmt.Sprintf("%s/%s", table, chain)
	// In a real iptables setup, the OUTPUT chain always exists.
	if _, exists := m.Chains[key]; !exists && chain == "OUTPUT" {
		m.Chains[key] = []string{"-N OUTPUT"}
	} else if !exists {
		return fmt.Errorf("chain `%s` does not exist in table `%s`", chain, table)
	}

	// Simulate the format of an iptables command, e.g., "-A OUTPUT -j LBCHAIN"
	rule := fmt.Sprintf("-A %s %s", chain, strings.Join(rulespec, " "))
	m.Chains[key] = append(m.Chains[key], rule)
	return nil
}

func (m *mockIPTables) ClearChain(table, chain string) error {
	key := fmt.Sprintf("%s/%s", table, chain)
	if _, exists := m.Chains[key]; !exists {
		return fmt.Errorf("chain `%s` does not exist in table `%s`", chain, table)
	}
	// Clearing a chain keeps the chain definition but removes all rules.
	m.Chains[key] = m.Chains[key][:1]
	return nil
}

func (m *mockIPTables) Delete(table, chain string, rulespec ...string) error {
	key := fmt.Sprintf("%s/%s", table, chain)
	if _, exists := m.Chains[key]; !exists {
		return fmt.Errorf("chain `%s` does not exist in table `%s`", chain, table)
	}

	ruleToDelete := fmt.Sprintf("-A %s %s", chain, strings.Join(rulespec, " "))
	var newRules []string
	found := false
	for _, rule := range m.Chains[key] {
		if rule == ruleToDelete && !found {
			found = true // only delete the first matching rule
			continue
		}
		newRules = append(newRules, rule)
	}

	if !found {
		return fmt.Errorf("rule `%s` not found in chain `%s`", ruleToDelete, chain)
	}
	m.Chains[key] = newRules
	return nil
}

func (m *mockIPTables) DeleteChain(table, chain string) error {
	key := fmt.Sprintf("%s/%s", table, chain)
	if _, exists := m.Chains[key]; !exists {
		return fmt.Errorf("chain `%s` does not exist in table `%s`", chain, table)
	}
	delete(m.Chains, key)
	return nil
}

// --- Unit Test Functions ---

// TestGetRandomBalancingProbability tests the logic of the pure function for probability calculation.
func TestGetRandomBalancingProbability(t *testing.T) {
	im := &IptablesManager{}
	testCases := []struct {
		name     string
		numOfIPs int
		expected []float64
	}{
		{"Zero IPs", 0, []float64{}},
		{"One IP", 1, []float64{1.0}},
		{"Three IPs", 3, []float64{1.0 / 3.0, 1.0 / 2.0, 1.0}},
		{"Four IPs", 4, []float64{1.0 / 4.0, 1.0 / 3.0, 1.0 / 2.0, 1.0}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := im.getRamdomBalancingProbability(tc.numOfIPs)
			assert.InDeltaSlice(t, tc.expected, actual, 0.00001, "Calculated probabilities do not match expected values")
		})
	}
}

// TestCleanIptablesRules tests the logic for cleaning up iptables rules.
func TestCleanIptablesRules(t *testing.T) {
	t.Run("Should successfully clean up when the chain exists", func(t *testing.T) {
		mockIpt := newMockIPTables()
		require.NoError(t, mockIpt.NewChain("nat", LBCHAIN))
		require.NoError(t, mockIpt.Append("nat", "OUTPUT", "-j", LBCHAIN))
		im := newIptablesManagerWithClient(mockIpt)

		err := im.cleanIptablesRules()

		require.NoError(t, err)
		_, exists := mockIpt.Chains["nat/LBCHAIN"]
		assert.False(t, exists, "Expected LBCHAIN to be deleted")
		assert.NotContains(t, mockIpt.Chains["nat/OUTPUT"], "-A OUTPUT -j LBCHAIN", "Expected the jump rule in the OUTPUT chain to be deleted")
	})

	t.Run("Should not return an error when the chain does not exist", func(t *testing.T) {
		mockIpt := newMockIPTables()
		im := newIptablesManagerWithClient(mockIpt)
		err := im.cleanIptablesRules()
		require.NoError(t, err)
	})
}

// TestAddIptablesRules tests the logic for adding iptables rules.
func TestAddIptablesRules(t *testing.T) {
	const service = "10.0.0.1:6443"
	apiservers := []string{"192.168.0.2:6443", "192.168.0.3:6443"}

	t.Run("Should create the chain and all rules when the chain does not exist", func(t *testing.T) {
		mockIpt := newMockIPTables()
		im := newIptablesManagerWithClient(mockIpt)
		err := im.addIptablesRules(service, apiservers)
		require.NoError(t, err)
		_, exists := mockIpt.Chains["nat/LBCHAIN"]
		assert.True(t, exists, "Expected LBCHAIN to be created")
		assert.Contains(t, mockIpt.Chains["nat/OUTPUT"], "-A OUTPUT -j LBCHAIN", "Expected OUTPUT chain to jump to LBCHAIN")

		lbchainRules := mockIpt.Chains["nat/LBCHAIN"]
		require.Len(t, lbchainRules, 3) // 1 definition + 2 rules
		assert.True(t, strings.Contains(lbchainRules[1], "--probability 0.5") && strings.Contains(lbchainRules[1], "--to-destination 192.168.0.2:6443"))
		assert.True(t, strings.Contains(lbchainRules[2], "--probability 1") && strings.Contains(lbchainRules[2], "--to-destination 192.168.0.3:6443"))
	})

	t.Run("Should only add DNAT rules when the chain already exists", func(t *testing.T) {
		mockIpt := newMockIPTables()
		require.NoError(t, mockIpt.NewChain("nat", LBCHAIN))
		im := newIptablesManagerWithClient(mockIpt)

		err := im.addIptablesRules(service, apiservers)
		require.NoError(t, err)

		lbchainRules := mockIpt.Chains["nat/LBCHAIN"]
		require.Len(t, lbchainRules, 3)
		outputRules := mockIpt.Chains["nat/OUTPUT"]
		assert.NotContains(t, outputRules, "-A OUTPUT -j LBCHAIN", "Should not add the jump rule again when the chain already exists")
	})

	t.Run("With an invalid service string", func(t *testing.T) {
		mockIpt := newMockIPTables()
		im := newIptablesManagerWithClient(mockIpt)
		err := im.addIptablesRules("this is not a valid address", apiservers)
		assert.Error(t, err, "Should return an error for an invalid service address")
	})
}

// TestUpdateIptablesRules tests the complete update process (clean then add).
func TestUpdateIptablesRules(t *testing.T) {
	// Arrange: Create a mock client with pre-existing old rules.
	mockIpt := newMockIPTables()
	require.NoError(t, mockIpt.NewChain("nat", LBCHAIN))
	require.NoError(t, mockIpt.Append("nat", "OUTPUT", "-j", LBCHAIN))
	require.NoError(t, mockIpt.Append("nat", LBCHAIN, "-j", "DNAT", "--to-destination", "1.1.1.1:6443"))
	im := newIptablesManagerWithClient(mockIpt)

	service := "10.0.0.1:6443"
	newApiservers := []string{"192.168.10.2:6443", "192.168.10.3:6443"}
	err := im.updateIptablesRules(service, newApiservers)
	require.NoError(t, err)
	lbchainRules := mockIpt.Chains["nat/LBCHAIN"]
	ruleStr := strings.Join(lbchainRules, "\n")

	assert.NotContains(t, ruleStr, "1.1.1.1:6443", "Old rules should have been cleaned up")
	assert.Contains(t, ruleStr, "192.168.10.2:6443", "Should contain the first new rule")
	assert.Contains(t, ruleStr, "192.168.10.3:6443", "Should contain the second new rule")
}
