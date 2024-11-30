package yurtcoordinator

import (
	"testing"
)

func TestFakeCoordinator_Run(t *testing.T) {
	fc := &FakeCoordinator{}
	fc.Run()
	// Add your assertions here
}
func TestFakeCoordinator_IsReady(t *testing.T) {
	fc := &FakeCoordinator{}
	_, _ = fc.IsReady()

	// Add your assertions here
}

func TestFakeCoordinator_IsHealthy(t *testing.T) {
	fc := &FakeCoordinator{}
	_, _ = fc.IsHealthy()

	// Add your assertions here
}
