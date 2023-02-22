/*
Copyright 2022 The OpenYurt Authors.

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
	"net/http"
	"reflect"
	"testing"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestNewMiddlewareInitializer(t *testing.T) {
	fc := informers.NewSharedInformerFactoryWithOptions(&fake.Clientset{}, 1)
	tests := []struct {
		name    string
		factory informers.SharedInformerFactory
		expect  MiddlewareInitializer
	}{
		{
			"normal",
			fc,
			&middlewareInitializer{
				factory: fc,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := NewMiddlewareInitializer(tt.factory)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}

type fhw struct {
}

func (f fhw) WrapHandler(http.Handler) http.Handler {
	return nil
}

func (f fhw) Name() string {
	return "fhw"
}

func (f fhw) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	_ = factory
	return nil
}

func TestInitializer(t *testing.T) {
	f := fhw{}
	fc := informers.NewSharedInformerFactoryWithOptions(&fake.Clientset{}, 1)
	mi := &middlewareInitializer{
		factory: fc,
	}

	tests := []struct {
		name   string
		m      handlerwrapper.Middleware
		expect error
	}{
		{
			"normal",
			f,
			nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := mi.Initialize(f)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}
