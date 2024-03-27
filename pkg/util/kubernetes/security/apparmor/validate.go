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
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	v1 "k8s.io/api/core/v1"
	utilpath "k8s.io/utils/path"
)

// Whether AppArmor should be disabled by default.
// Set to true if the wrong build tags are set (see validate_disabled.go).

// Whether AppArmor should be disabled by default.
// Set to true if the wrong build tags are set (see validate_disabled.go).
var isDisabledBuild bool

// Verify that the profile is valid and loaded.
func validateProfile(profile string, loadedProfiles map[string]bool) error {
	if err := ValidateProfileFormat(profile); err != nil {
		return err
	}

	if strings.HasPrefix(profile, v1.AppArmorBetaProfileNamePrefix) {
		profileName := strings.TrimPrefix(profile, v1.AppArmorBetaProfileNamePrefix)
		if !loadedProfiles[profileName] {
			return fmt.Errorf("profile %q is not loaded", profileName)
		}
	}

	return nil
}

// ValidateProfileFormat checks the format of the profile.
func ValidateProfileFormat(profile string) error {
	if profile == "" || profile == v1.AppArmorBetaProfileRuntimeDefault || profile == v1.AppArmorBetaProfileNameUnconfined {
		return nil
	}
	if !strings.HasPrefix(profile, v1.AppArmorBetaProfileNamePrefix) {
		return fmt.Errorf("invalid AppArmor profile name: %q", profile)
	}
	return nil
}

// The profiles file is formatted with one profile per line, matching a form:
//
//	namespace://profile-name (mode)
//	profile-name (mode)
//
// Where mode is {enforce, complain, kill}. The "namespace://" is only included for namespaced
// profiles. For the purposes of Kubernetes, we consider the namespace part of the profile name.
func parseProfileName(profileLine string) string {
	modeIndex := strings.IndexRune(profileLine, '(')
	if modeIndex < 0 {
		return ""
	}
	return strings.TrimSpace(profileLine[:modeIndex])
}

func getAppArmorFS() (string, error) {
	mountsFile, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("could not open /proc/mounts: %v", err)
	}
	defer mountsFile.Close()

	scanner := bufio.NewScanner(mountsFile)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			// Unknown line format; skip it.
			continue
		}
		if fields[2] == "securityfs" {
			appArmorFS := path.Join(fields[1], "apparmor")
			if ok, err := utilpath.Exists(utilpath.CheckFollowSymlink, appArmorFS); !ok {
				msg := fmt.Sprintf("path %s does not exist", appArmorFS)
				if err != nil {
					return "", fmt.Errorf("%s: %v", msg, err)
				}
				return "", errors.New(msg)
			}
			return appArmorFS, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning mounts: %v", err)
	}

	return "", errors.New("securityfs not found")
}
