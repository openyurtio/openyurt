/*
Copyright 2023 The OpenYurt Authors.

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

package resources

import "k8s.io/apimachinery/pkg/runtime/schema"

type verifiablePoolScopeResource struct {
	schema.GroupVersionResource
	checkFunction func(gvr schema.GroupVersionResource) (bool, string)
}

func newVerifiablePoolScopeResource(gvr schema.GroupVersionResource,
	checkFunction func(gvr schema.GroupVersionResource) (bool, string)) *verifiablePoolScopeResource {
	return &verifiablePoolScopeResource{
		GroupVersionResource: gvr,
		checkFunction:        checkFunction,
	}
}

func (v *verifiablePoolScopeResource) Verify() (bool, string) {
	return v.checkFunction(v.GroupVersionResource)
}
