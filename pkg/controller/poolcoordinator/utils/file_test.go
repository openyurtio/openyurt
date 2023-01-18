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

package utils

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	r := GetEnv("", "a")
	if r != "a" {
		t.Fail()
	}
	r = GetEnv("HOME", "")
	if r == "" {
		t.Fail()
	}
}

func TestFileExists(t *testing.T) {
	WriteFile("/tmp/abcd", []byte("abcd"))
	if FileExists("/tmp/abcd") != true {
		t.Fail()
	}
	os.Remove("/tmp/abcd")
	if FileExists("/dev/abcd") == true {
		t.Fail()
	}
}

func TestDirExists(t *testing.T) {
	if DirExists("/tmp") != true {
		t.Fail()
	}
}

func TestEnsureDir(t *testing.T) {
	if EnsureDir("/tmp") != nil {
		t.Fail()
	}
}
