package network

import (
	"net"
	"testing"
)

func TestEnsureDummyInterface(t *testing.T) {
	// Create a new instance of the dummyInterfaceController
	dic := NewDummyInterfaceController()

	// Define the test input values
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")

	// Call the EnsureDummyInterface function
	err := dic.EnsureDummyInterface(ifName, ifIP)

	// Check if the function returned an error
	if err != nil {
		t.Errorf("EnsureDummyInterface returned an error: %v", err)
	}

	// TODO: Add additional assertions to verify the behavior of EnsureDummyInterface
}
func TestEnsureDummyInterface_ExistingInterfaceAndMatchingIP(t *testing.T) {
	dic := NewDummyInterfaceController()

	// Create a dummy interface with the specified name and IP
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")
	err := dic.EnsureDummyInterface(ifName, ifIP)
	if err != nil {
		t.Fatalf("Failed to create dummy interface: %v", err)
	}

	// Call the EnsureDummyInterface function
	err = dic.EnsureDummyInterface(ifName, ifIP)

	// Check if the function returned nil error
	if err != nil {
		t.Errorf("EnsureDummyInterface returned an error: %v", err)
	}
}

func TestEnsureDummyInterface_ExistingInterfaceAndDifferentIP(t *testing.T) {
	dic := NewDummyInterfaceController()

	// Create a dummy interface with the specified name and IP
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")
	err := dic.EnsureDummyInterface(ifName, ifIP)
	if err != nil {
		t.Fatalf("Failed to create dummy interface: %v", err)
	}

	// Call the EnsureDummyInterface function with a different IP
	newIP := net.ParseIP("192.168.0.2")
	err = dic.EnsureDummyInterface(ifName, newIP)

	// Check if the function returned nil error
	if err != nil {
		t.Errorf("EnsureDummyInterface returned an error: %v", err)
	}
}

func TestEnsureDummyInterface_NonExistingInterface(t *testing.T) {
	dic := NewDummyInterfaceController()

	// Define the test input values
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")

	// Call the EnsureDummyInterface function
	err := dic.EnsureDummyInterface(ifName, ifIP)

	// Check if the function returned nil error
	if err != nil {
		t.Errorf("EnsureDummyInterface returned an error: %v", err)
	}
}

func TestDeleteDummyInterface(t *testing.T) {
	dic := NewDummyInterfaceController()

	// Create a dummy interface with the specified name and IP
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")
	err := dic.EnsureDummyInterface(ifName, ifIP)
	if err != nil {
		t.Fatalf("Failed to create dummy interface: %v", err)
	}

	// Call the DeleteDummyInterface function
	err = dic.DeleteDummyInterface(ifName)

	// Check if the function returned nil error
	if err != nil {
		t.Errorf("DeleteDummyInterface returned an error: %v", err)
	}
}

func TestListDummyInterface(t *testing.T) {
	dic := NewDummyInterfaceController()

	// Create a dummy interface with the specified name and IP
	ifName := "dummy0"
	ifIP := net.ParseIP("192.168.0.1")
	err := dic.EnsureDummyInterface(ifName, ifIP)
	if err != nil {
		t.Fatalf("Failed to create dummy interface: %v", err)
	}

	// Call the ListDummyInterface function
	ips, err := dic.ListDummyInterface(ifName)

	// Check if the function returned nil error
	if err != nil {
		t.Errorf("ListDummyInterface returned an error: %v", err)
	}

	// Check if the returned IPs match the expected IP
	if len(ips) != 1 || !ips[0].Equal(ifIP) {
		t.Errorf("ListDummyInterface returned unexpected IPs: %v", ips)
	}
}
