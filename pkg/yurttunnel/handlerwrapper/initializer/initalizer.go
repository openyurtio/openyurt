/*
Copyright 2020 The OpenYurt Authors.

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

package initializer

import (
	"k8s.io/client-go/informers"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
)

// WantsSharedInformerFactory is an interface for setting SharedInformerFactory
type WantsSharedInformerFactory interface {
	SetSharedInformerFactory(factory informers.SharedInformerFactory) error
}

// MiddlewareInitializer is an interface for initializing middleware
type MiddlewareInitializer interface {
	Initialize(m handlerwrapper.Middleware) error
}

// middlewareInitializer is responsible for initializing middleware
type middlewareInitializer struct {
	factory informers.SharedInformerFactory
}

// NewMiddlewareInitializer creates an MiddlewareInitializer object
func NewMiddlewareInitializer(factory informers.SharedInformerFactory) MiddlewareInitializer {
	return &middlewareInitializer{
		factory: factory,
	}
}

// Initialize used for executing middleware initialization
func (mi *middlewareInitializer) Initialize(m handlerwrapper.Middleware) error {
	if wants, ok := m.(WantsSharedInformerFactory); ok {
		if err := wants.SetSharedInformerFactory(mi.factory); err != nil {
			return err
		}
	}

	return nil
}
