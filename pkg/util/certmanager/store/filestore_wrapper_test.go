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

package store

import (
	"reflect"
	"testing"

	"k8s.io/client-go/util/certificate"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

var fw, _ = NewFileStoreWrapper("", "", "", "", "")

func TestNewFileStoreWrapper(t *testing.T) {
	tests := []struct {
		name           string
		pairNamePrefix string
		certDirectory  string
		keyDirectory   string
		certFile       string
		keyFile        string
		expect         error
	}{
		{
			"normal",
			"",
			"",
			"",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := NewFileStoreWrapper(
					tt.pairNamePrefix,
					tt.certDirectory,
					tt.keyDirectory,
					tt.certFile,
					tt.keyFile)

				if !reflect.DeepEqual(get, tt.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestCurrent(t *testing.T) {
	noCertKeyErr := certificate.NoCertKeyError("NO_VALID_CERT")
	tests := []struct {
		name   string
		expect error
	}{
		{
			"normal",
			&noCertKeyErr,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := fw.Current()

				if !reflect.DeepEqual(get, tt.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}
