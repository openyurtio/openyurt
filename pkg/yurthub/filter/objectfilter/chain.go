/*
Copyright 2024 The OpenYurt Authors.

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

package objectfilter

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type filterChain []filter.ObjectFilter

func CreateFilterChain(objFilters []filter.ObjectFilter) filter.ObjectFilter {
	chain := make(filterChain, 0)
	chain = append(chain, objFilters...)
	return chain
}

func (chain filterChain) Name() string {
	var names []string
	for i := range chain {
		names = append(names, chain[i].Name())
	}
	return strings.Join(names, ",")
}

func (chain filterChain) SupportedResourceAndVerbs() map[string]sets.Set[string] {
	// do nothing
	return map[string]sets.Set[string]{}
}

func (chain filterChain) Filter(obj runtime.Object) runtime.Object {
	for i := range chain {
		obj = chain[i].Filter(obj)
		if yurtutil.IsNil(obj) {
			break
		}
	}

	return obj
}
