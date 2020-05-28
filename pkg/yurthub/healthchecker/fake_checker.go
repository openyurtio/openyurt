package healthchecker

import (
	"net/url"
)

type fakeChecker struct {
	healthy  bool
	settings map[string]int
}

// IsHealthy returns healthy status of server
func (fc *fakeChecker) IsHealthy(server *url.URL) bool {
	s := server.String()
	if _, ok := fc.settings[s]; !ok {
		return fc.healthy
	}

	if fc.settings[s] < 0 {
		return fc.healthy
	}

	if fc.settings[s] == 0 {
		return !fc.healthy
	}

	fc.settings[s] = fc.settings[s] - 1
	return fc.healthy
}

// NewFakeChecker creates a fake checker
func NewFakeChecker(healthy bool, settings map[string]int) HealthChecker {
	return &fakeChecker{
		settings: settings,
		healthy:  healthy,
	}
}
