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

package options

import (
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter/base"
)

func TestRegisterAllFilters(t *testing.T) {
	disableFilter := "servicetopology"
	filters := base.NewFilters([]string{disableFilter})
	RegisterAllFilters(filters)
	if filters.Enabled(disableFilter) {
		t.Errorf("expect %s disable, but it is enabled", disableFilter)
	}
}
