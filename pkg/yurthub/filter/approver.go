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

package filter

import "k8s.io/apimachinery/pkg/util/sets"

type Approver struct {
	comp       string
	resource   string
	operations sets.String
}

func NewApprover(comp, resource string, verbs ...string) *Approver {
	return &Approver{
		comp:       comp,
		resource:   resource,
		operations: sets.NewString(verbs...),
	}
}

func (a *Approver) Approve(comp, resource, verb string) bool {
	if a.comp != comp || a.resource != resource {
		return false
	}

	return a.operations.Has(verb)
}
