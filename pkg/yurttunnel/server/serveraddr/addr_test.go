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
package serveraddr

import (
	"fmt"
	"testing"
)

func TestGetDefaultDomainsForSvcInputParamEmptyChar(t *testing.T) {
	domains := getDefaultDomainsForSvc("", "")
	if len(domains) != 0 {
		t.Error("domains len is not equal zero")
	}
}

func TestGetDefaultDomainsForSvc(t *testing.T) {
	ns := "hello"
	name := "world"
	domains := getDefaultDomainsForSvc(ns, name)
	if len(domains) == 0 {
		t.Log("domains len is zero")
	} else {
		if len(domains) == 4 {
			if name != domains[0] {
				t.Errorf("The two words should be the same:%s\n", name)
			}
			if fmt.Sprintf("%s.%s", name, ns) != domains[1] {
				t.Errorf("The two words should be the same,%s.%s\n", name, ns)
			}
			if fmt.Sprintf("%s.%s.svc", name, ns) != domains[2] {
				t.Errorf("The two words should be the same,%s.%s.svc", name, ns)
			}
			if fmt.Sprintf("%s.%s.svc.cluster.local", name, ns) != domains[3] {
				t.Errorf("The two words should be the same,%s.%s.svc.cluster.local", name, ns)
			}
		}
	}
}
