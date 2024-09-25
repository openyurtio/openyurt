package network

import (
	"testing"
	"time"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
)

// TestNewNetworkManager tests the creation of a new NetworkManager.
func TestNewNetworkManager(t *testing.T) {
	opts := &options.YurtHubOptions{
		HubAgentDummyIfIP:      "127.0.0.1",
		YurtHubProxyPort:       10261,
		YurtHubProxySecurePort: 10262,
		HubAgentDummyIfName:    "dummy0",
		EnableIptables:         true,
	}
	nm, err := NewNetworkManager(opts)
	if err != nil {
		t.Fatalf("Failed to create NetworkManager: %v", err)
	}
	if nm.dummyIfIP.String() != opts.HubAgentDummyIfIP {
		t.Errorf("Expected dummyIfIP %s, got %s", opts.HubAgentDummyIfIP, nm.dummyIfIP.String())
	}
	if nm.dummyIfName != opts.HubAgentDummyIfName {
		t.Errorf("Expected dummyIfName %s, got %s", opts.HubAgentDummyIfName, nm.dummyIfName)
	}
	if nm.enableIptables != opts.EnableIptables {
		t.Errorf("Expected enableIptables %t, got %t", opts.EnableIptables, nm.enableIptables)
	}
}

// TestRun tests the Run method of NetworkManager.
func TestRun(t *testing.T) {
	opts := &options.YurtHubOptions{
		HubAgentDummyIfIP:      "127.0.0.1",
		YurtHubProxyPort:       10261,
		YurtHubProxySecurePort: 10262,
		HubAgentDummyIfName:    "dummy0",
		EnableIptables:         true,
	}
	nm, _ := NewNetworkManager(opts)
	stopCh := make(chan struct{})
	go nm.Run(stopCh)
	time.Sleep(1 * time.Second) // Wait for the goroutine to start
	close(stopCh)               // Send stop signal
	// Test is successful if it doesn't hang and exits correctly
}

// TestConfigureNetwork tests the configureNetwork method.
func TestConfigureNetwork(t *testing.T) {
	opts := &options.YurtHubOptions{
		HubAgentDummyIfIP:      "127.0.0.1",
		YurtHubProxyPort:       10261,
		YurtHubProxySecurePort: 10262,
		HubAgentDummyIfName:    "dummy0",
		EnableIptables:         true,
	}
	nm, _ := NewNetworkManager(opts)
	err := nm.configureNetwork()
	if err != nil {
		t.Errorf("Failed to configure network: %v", err)
	}
	// Further checks can be added to verify the configuration
}
