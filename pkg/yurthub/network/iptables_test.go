package network

import (
	"testing"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
	"github.com/stretchr/testify/assert"
)

func TestNewIptablesManager(t *testing.T) {
	// Test the creation of a new IptablesManager
	dummyIP := "169.254.2.1"
	dummyPort := "10261"
	manager := NewIptablesManager(dummyIP, dummyPort)
	assert.NotNil(t, manager)
}
func TestMakeupIptablesRules(t *testing.T) {
	ifIP := "169.254.2.1"
	ifPort := "10261"

	expectedRules := []iptablesRule{
		{iptables.Prepend, iptables.TableFilter, iptables.ChainInput, []string{"-p", "tcp", "-m", "comment", "--comment", "for container access hub agent", "--dport", ifPort, "--destination", ifIP, "-j", "ACCEPT"}},
		{iptables.Prepend, iptables.TableFilter, iptables.ChainOutput, []string{"-p", "tcp", "--sport", ifPort, "-s", ifIP, "-j", "ACCEPT"}},
	}

	actualRules := makeupIptablesRules(ifIP, ifPort)

	assert.Equal(t, expectedRules, actualRules)
}
func TestEnsureIptablesRules(t *testing.T) {
	manager := NewIptablesManager("169.254.2.1", "10261")
	err := manager.EnsureIptablesRules()
	assert.NoError(t, err)
}
func TestCleanUpIptablesRules(t *testing.T) {
	manager := NewIptablesManager("169.254.2.1", "10261")
	err := manager.CleanUpIptablesRules()
	assert.NoError(t, err)
}
