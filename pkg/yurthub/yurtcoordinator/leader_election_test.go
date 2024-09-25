package yurtcoordinator

import (
	"testing"
)

func TestHubElector_StatusChan(t *testing.T) {
	he := &HubElector{
		electorStatus: make(chan int32, 1),
	}

	statusChan := he.StatusChan()

	// Send a value to the status channel
	expectedStatus := int32(LeaderHub)
	he.electorStatus <- expectedStatus

	// Receive the value from the status channel
	receivedStatus := <-statusChan

	// Check if the received status matches the expected status
	if receivedStatus != expectedStatus {
		t.Errorf("Received status %d, expected %d", receivedStatus, expectedStatus)
	}
}
