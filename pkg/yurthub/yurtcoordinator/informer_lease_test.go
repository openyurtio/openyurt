package yurtcoordinator

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestNewInformerLease(t *testing.T) {
	// Create a fake clientset
	client := fake.NewSimpleClientset()

	// Call the NewInformerLease function
	lease := NewInformerLease(client, "test-lease", "test-namespace", "test-identity", 60, 3)

	// Assert that the lease is not nil
	if lease == nil {
		t.Error("Expected lease to be created, but got nil")
	}

	// Add more assertions here if needed
}
