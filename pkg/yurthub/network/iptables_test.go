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

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
)

func TestEnsureIptablesRules(t *testing.T) {
	mockIptables := new(mockIptablesInterface)
	im := &IptablesManager{
		iptables: mockIptables,
		rules:    makeupIptablesRules("169.254.2.1", "10261"),
	}

	mockIptables.On("EnsureRule", iptables.Prepend, iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", "10261", "--destination", "169.254.2.1", "-j", "ACCEPT"}).Return(true, nil)
	mockIptables.On("EnsureRule", iptables.Prepend, iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", "10261", "-s", "169.254.2.1", "-j", "ACCEPT"}).Return(true, nil)

	err := im.EnsureIptablesRules()
	assert.NoError(t, err)
	mockIptables.AssertExpectations(t)
}

func TestEnsureIptablesRulesWithError(t *testing.T) {
	mockIptables := new(mockIptablesInterface)
	im := &IptablesManager{
		iptables: mockIptables,
		rules:    makeupIptablesRules("169.254.2.1", "10261"),
	}

	mockIptables.On("EnsureRule", iptables.Prepend, iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", "10261", "--destination", "169.254.2.1", "-j", "ACCEPT"}).Return(false, assert.AnError)
	mockIptables.On("EnsureRule", iptables.Prepend, iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", "10261", "-s", "169.254.2.1", "-j", "ACCEPT"}).Return(false, assert.AnError)

	err := im.EnsureIptablesRules()
	assert.Error(t, err)
	mockIptables.AssertExpectations(t)
}

func TestCleanUpIptablesRules(t *testing.T) {
	mockIptables := new(mockIptablesInterface)
	im := &IptablesManager{
		iptables: mockIptables,
		rules:    makeupIptablesRules("169.254.2.1", "10261"),
	}

	mockIptables.On("DeleteRule", iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", "10261", "--destination", "169.254.2.1", "-j", "ACCEPT"}).Return(nil)
	mockIptables.On("DeleteRule", iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", "10261", "-s", "169.254.2.1", "-j", "ACCEPT"}).Return(nil)

	err := im.CleanUpIptablesRules()
	assert.NoError(t, err)
	mockIptables.AssertExpectations(t)
}

func TestCleanUpIptablesRulesWithError(t *testing.T) {
	mockIptables := new(mockIptablesInterface)
	im := &IptablesManager{
		iptables: mockIptables,
		rules:    makeupIptablesRules("169.254.2.1", "10261"),
	}

	mockIptables.On("DeleteRule", iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", "10261", "--destination", "169.254.2.1", "-j", "ACCEPT"}).Return(assert.AnError)
	mockIptables.On("DeleteRule", iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", "10261", "-s", "169.254.2.1", "-j", "ACCEPT"}).Return(assert.AnError)

	err := im.CleanUpIptablesRules()
	assert.Error(t, err)
	mockIptables.AssertExpectations(t)
}

type mockIptablesInterface struct {
	mock.Mock
	iptables.Interface
}

func (m *mockIptablesInterface) EnsureRule(position iptables.RulePosition, table iptables.Table, chain iptables.Chain, args ...string) (bool, error) {
	argsList := m.Called(position, table, chain, args)
	return argsList.Bool(0), argsList.Error(1)
}

func (m *mockIptablesInterface) DeleteRule(table iptables.Table, chain iptables.Chain, args ...string) error {
	argsList := m.Called(table, chain, args)
	return argsList.Error(0)
}
