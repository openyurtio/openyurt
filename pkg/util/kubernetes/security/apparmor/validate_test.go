/*
Copyright 2016 The Kubernetes Authors.

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

package apparmor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestGetAppArmorFS(t *testing.T) {
	// This test only passes on systems running AppArmor with the default configuration.
	// The test should be manually run if modifying the getAppArmorFS function.
	t.Skip()

	const expectedPath = "/sys/kernel/security/apparmor"
	actualPath, err := getAppArmorFS()
	assert.NoError(t, err)
	assert.Equal(t, expectedPath, actualPath)
}

func TestValidateProfile(t *testing.T) {
	loadedProfiles := map[string]bool{
		"docker-default": true,
		"foo-bar":        true,
		"baz":            true,
		"/usr/sbin/ntpd": true,
		"/usr/lib/connman/scripts/dhclient-script":      true,
		"/usr/lib/NetworkManager/nm-dhcp-client.action": true,
		"/usr/bin/evince-previewer//sanitized_helper":   true,
	}
	tests := []struct {
		profile     string
		expectValid bool
	}{
		{"", true},
		{v1.AppArmorBetaProfileRuntimeDefault, true},
		{v1.AppArmorBetaProfileNameUnconfined, true},
		{"baz", false}, // Missing local prefix.
		{v1.AppArmorBetaProfileNamePrefix + "/usr/sbin/ntpd", true},
		{v1.AppArmorBetaProfileNamePrefix + "foo-bar", true},
		{v1.AppArmorBetaProfileNamePrefix + "unloaded", false}, // Not loaded.
		{v1.AppArmorBetaProfileNamePrefix + "", false},
	}

	for _, test := range tests {
		err := validateProfile(test.profile, loadedProfiles)
		if test.expectValid {
			assert.NoError(t, err, "Profile %s should be valid", test.profile)
		} else {
			assert.Error(t, err, fmt.Sprintf("Profile %s should not be valid", test.profile))
		}
	}
}

func TestParseProfileName(t *testing.T) {
	tests := []struct{ line, expected string }{
		{"foo://bar/baz (kill)", "foo://bar/baz"},
		{"foo-bar (enforce)", "foo-bar"},
		{"/usr/foo/bar/baz (complain)", "/usr/foo/bar/baz"},
	}
	for _, test := range tests {
		name := parseProfileName(test.line)
		assert.Equal(t, test.expected, name, "Parsing %s", test.line)
	}
}
