/*
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

package options

import (
	"fmt"
	"net"

	"k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
)

// InsecureServingOptions are for creating an unauthenticated, unauthorized, insecure port.
// No one should be using these anymore.
type InsecureServingOptions struct {
	BindAddress net.IP
	BindPort    int
	// BindNetwork is the type of network to bind to - defaults to "tcp", accepts "tcp",
	// "tcp4", and "tcp6".
	BindNetwork string

	// Listener is the secure server network listener.
	// either Listener or BindAddress/BindPort/BindNetwork is set,
	// if Listener is set, use it and omit BindAddress/BindPort/BindNetwork.
	Listener net.Listener

	// ListenFunc can be overridden to create a custom listener, e.g. for mocking in tests.
	// It defaults to options.CreateListener.
	ListenFunc func(network, addr string, config net.ListenConfig) (net.Listener, int, error)
}

// ApplyTo adds InsecureServingOptions to the insecureserverinfo.
// Note: the double pointer allows to set the *InsecureServingInfo to nil without referencing the struct hosting this pointer.
func (s *InsecureServingOptions) ApplyTo(c **server.DeprecatedInsecureServingInfo) error {
	if s == nil {
		return nil
	}
	if s.BindPort <= 0 {
		return nil
	}

	if s.Listener == nil {
		var err error
		listen := options.CreateListener
		if s.ListenFunc != nil {
			listen = s.ListenFunc
		}
		addr := net.JoinHostPort(s.BindAddress.String(), fmt.Sprintf("%d", s.BindPort))
		s.Listener, s.BindPort, err = listen(s.BindNetwork, addr, net.ListenConfig{})
		if err != nil {
			return fmt.Errorf("failed to create listener: %v", err)
		}
	}

	*c = &server.DeprecatedInsecureServingInfo{
		Listener: s.Listener,
	}

	return nil
}
