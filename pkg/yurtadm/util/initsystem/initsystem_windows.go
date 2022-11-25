//go:build windows
// +build windows

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
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// WindowsInitSystem is the windows implementation of InitSystem
type WindowsInitSystem struct{}

// ServiceIsActive ensures the service is running, or attempting to run. (crash looping in the case of kubelet)
func (sysd WindowsInitSystem) ServiceIsActive(service string) bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(service)
	if err != nil {
		return false
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false
	}
	return status.State == svc.Running
}

// GetInitSystem returns an InitSystem for the current system, or nil
// if we cannot detect a supported init system.
// This indicates we will skip init system checks, not an error.
func GetInitSystem() (InitSystem, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("no supported init system detected: %v", err)
	}
	defer m.Disconnect()
	return &WindowsInitSystem{}, nil
}
