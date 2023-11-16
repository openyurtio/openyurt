//go:build !windows
// +build !windows

/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package initsystem

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// OpenRCInitSystem defines openrc
type OpenRCInitSystem struct{}

// ServiceIsEnabled ensures the service is enabled to start on each boot.
func (openrc OpenRCInitSystem) ServiceIsEnabled(service string) bool {
	args := []string{"show", "default"}
	outBytes, _ := exec.Command("rc-update", args...).Output()
	return strings.Contains(string(outBytes), service)
}

// ServiceEnable tries to start a specific service
func (openrc OpenRCInitSystem) ServiceEnable(service string) error {
	args := []string{"add", service, "default"}
	return exec.Command("rc-update", args...).Run()
}

// ServiceIsActive ensures the service is running, or attempting to run. (crash looping in the case of kubelet)
func (openrc OpenRCInitSystem) ServiceIsActive(service string) bool {
	args := []string{service, "status"}
	outBytes, _ := exec.Command("rc-service", args...).CombinedOutput()
	outStr := string(outBytes)
	return !strings.Contains(outStr, "stopped") && !strings.Contains(outStr, "does not exist")
}

// SystemdInitSystem defines systemd
type SystemdInitSystem struct{}

// reloadSystemd reloads the systemd daemon
func (sysd SystemdInitSystem) reloadSystemd() error {
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "could not reload systemd")
	}
	return nil
}

// ServiceIsEnabled ensures the service is enabled to start on each boot.
func (sysd SystemdInitSystem) ServiceIsEnabled(service string) bool {
	args := []string{"is-enabled", service}
	err := exec.Command("systemctl", args...).Run()
	return err == nil
}

// ServiceEnable tries to start a specific service
func (sysd SystemdInitSystem) ServiceEnable(service string) error {
	// Before we try to start any service, make sure that systemd is ready
	if err := sysd.reloadSystemd(); err != nil {
		return err
	}
	args := []string{"enable", service}
	return exec.Command("systemctl", args...).Run()
}

// ServiceIsActive will check is the service is "active". In the case of
// crash looping services (kubelet in our case) status will return as
// "activating", so we will consider this active as well.
func (sysd SystemdInitSystem) ServiceIsActive(service string) bool {
	args := []string{"is-active", service}
	// Ignoring error here, command returns non-0 if in "activating" status:
	outBytes, _ := exec.Command("systemctl", args...).Output()
	output := strings.TrimSpace(string(outBytes))
	if output == "active" || output == "activating" {
		return true
	}
	return false
}

// GetInitSystem returns an InitSystem for the current system, or nil
// if we cannot detect a supported init system.
// This indicates we will skip init system checks, not an error.
func GetInitSystem() (InitSystem, error) {
	// Assume existence of systemctl in path implies this is a systemd system:
	_, err := exec.LookPath("systemctl")
	if err == nil {
		return &SystemdInitSystem{}, nil
	}
	_, err = exec.LookPath("openrc")
	if err == nil {
		return &OpenRCInitSystem{}, nil
	}

	return nil, fmt.Errorf("no supported init system detected, skipping checking for services")
}
