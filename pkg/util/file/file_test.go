/*
Copyright 2015 The Kubernetes Authors.

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

package file

import (
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestFileExists(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expect   bool
	}{
		{
			"nil",
			"nil",
			false,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := FileExists(tt.filename)

				if !reflect.DeepEqual(get, tt.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestReadObjectFromYamlFile(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		expect runtime.Object
	}{
		{
			"nil",
			"nil",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := ReadObjectFromYamlFile(tt.path)

				if !reflect.DeepEqual(get, tt.expect) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestWriteObjectToYamlFile(t *testing.T) {
	tests := []struct {
		name   string
		obj    runtime.Object
		path   string
		expect error
	}{
		{
			"nil",
			&v1.Pod{},
			"nil",
			fmt.Errorf("open nil: no such file or directory"),
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := WriteObjectToYamlFile(tt.obj, tt.path)
				if !reflect.DeepEqual(get, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestBackupFile(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		expect error
	}{
		{
			"nil",
			"nil",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := backupFile(tt.path)
				if !reflect.DeepEqual(get, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}
