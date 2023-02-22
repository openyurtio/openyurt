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

package wraphandler

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/handlerwrapper/initializer"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

type mdwi struct {
}

func (mdwi) Initialize(m handlerwrapper.Middleware) error {
	return nil
}

type tmdwi struct {
}

func (tmdwi) Initialize(m handlerwrapper.Middleware) error {
	if m.Name() == "TraceReqMiddleware" {
		return fmt.Errorf("TraceReqMiddleware error")
	}
	return nil
}

func TestInitHandlerWrappers(t *testing.T) {
	var a mdwi
	var b tmdwi
	tests := []struct {
		name   string
		mi     initializer.MiddlewareInitializer
		isIPv6 bool
		expect error
	}{
		{
			"normal",
			a,
			true,
			nil,
		},
		{
			"TraceReqMiddleware",
			b,
			true,
			fmt.Errorf("TraceReqMiddleware error"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := InitHandlerWrappers(tt.mi, tt.isIPv6)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}

type h struct {
}

func (h) ServeHTTP(http.ResponseWriter, *http.Request) {

}

func TestWrapHandler(t *testing.T) {
	var b h
	ewrappers := make(handlerwrapper.HandlerWrappers, 0)
	tests := []struct {
		name        string
		coreHandler http.Handler
		wrappers    handlerwrapper.HandlerWrappers
		expect      error
	}{
		{
			"empty",
			b,
			ewrappers,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := WrapHandler(tt.coreHandler, tt.wrappers)

				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}
