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

package gate

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestEnvEnabled(t *testing.T) {
	// save current function and retore in the end
	oldOSGetenv := osGetenv
	defer func() {
		osGetenv = oldOSGetenv
	}()

	var myEnv string

	tests := []struct {
		name   string
		gvk    schema.GroupVersionKind
		env    string
		expect bool
	}{
		{
			"all enabled by default when env is empty",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			"",
			true,
		},
		{
			"env not enabled",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			"test2",
			false,
		},
		{
			"env enabled",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			"test",
			true,
		},
	}

	osGetenv = func(key string) string {
		return myEnv
	}

	for _, st := range tests {
		tf := func(t *testing.T) {
			t.Logf("\tTestCase: %s", st.name)
			{
				myEnv = st.env
				get := envEnabled(st.gvk)
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}

}

func newFakeDiscovery(resources []*metav1.APIResourceList) discovery.DiscoveryInterface {
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery, _ := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscovery.Resources = resources
	return fakeDiscovery
}

func TestDiscoveryEnabled(t *testing.T) {
	oldClient := discoveryClient
	defer func() {
		discoveryClient = oldClient
	}()

	tests := []struct {
		name   string
		gvk    schema.GroupVersionKind
		client discovery.DiscoveryInterface
		expect bool
	}{
		{
			"nil client",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			nil,
			true,
		},
		{
			"not found by client (empty resources)",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			newFakeDiscovery(nil),
			// #73, this case should return `false` with the latest client-go version
			true,
		},
		{
			"not found by client (unmatch resources GV)",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			newFakeDiscovery([]*metav1.APIResourceList{
				{GroupVersion: "apps/v2"},
			}),
			true,
		},
		{
			"GV match, resources not found ",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			newFakeDiscovery([]*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Kind: "untest",
						},
					},
				},
			}),
			false,
		},
		{
			"GV match, resources found ",
			schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "test",
			},
			newFakeDiscovery([]*metav1.APIResourceList{
				{
					GroupVersion: "apps/v1",
					APIResources: []metav1.APIResource{
						{
							Kind: "test",
						},
					},
				},
			}),
			true,
		},
	}

	for _, st := range tests {
		tf := func(t *testing.T) {
			t.Logf("\tTestCase: %s", st.name)
			{
				discoveryClient = st.client
				get := discoveryEnabled(st.gvk)
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}

}
