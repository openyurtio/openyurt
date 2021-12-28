//go:build !linux
// +build !linux

/*
Copyright 2021 The OpenYurt Authors.

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

package network

import (
	"net"
)

type DummyInterfaceController interface {
	EnsureDummyInterface(ifName string, ifIP net.IP) error
	DeleteDummyInterface(ifName string) error
	ListDummyInterface(ifName string) ([]net.IP, error)
}

type unsupportedInterfaceController struct {
}

func NewDummyInterfaceController() DummyInterfaceController {
	return &unsupportedInterfaceController{}
}

// EnsureDummyInterface unimplemented
func (uic *unsupportedInterfaceController) EnsureDummyInterface(ifName string, ifIP net.IP) error {
	return nil
}

// DeleteDummyInterface unimplemented
func (uic *unsupportedInterfaceController) DeleteDummyInterface(ifName string) error {
	return nil
}

// ListDummyInterface unimplemented
func (dic *unsupportedInterfaceController) ListDummyInterface(ifName string) ([]net.IP, error) {
	return []net.IP{}, nil
}
