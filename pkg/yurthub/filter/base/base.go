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

package base

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type Factory func() (filter.ObjectFilter, error)

type Filters struct {
	sync.Mutex
	names           []string
	registry        map[string]Factory
	disabledFilters sets.String
}

func NewFilters(disabledFilters []string) *Filters {
	return &Filters{
		names:           make([]string, 0),
		registry:        make(map[string]Factory),
		disabledFilters: sets.NewString(disabledFilters...),
	}
}

func (fs *Filters) NewFromFilters(initializer filter.Initializer) ([]filter.ObjectFilter, error) {
	var filters = make([]filter.ObjectFilter, 0)
	for _, name := range fs.names {
		if fs.Enabled(name) {
			factory, found := fs.registry[name]
			if !found {
				return nil, fmt.Errorf("filter %s has not registered", name)
			}

			ins, err := factory()
			if err != nil {
				klog.Errorf("new filter %s failed, %v", name, err)
				return nil, err
			}

			if err = initializer.Initialize(ins); err != nil {
				klog.Errorf("Filter %s initialize failed, %v", name, err)
				return nil, err
			}
			klog.V(2).Infof("Filter %s initialize successfully", name)
			filters = append(filters, ins)
		} else {
			klog.V(2).Infof("Filter %s is disabled", name)
		}
	}

	return filters, nil
}

func (fs *Filters) Register(name string, fn Factory) {
	fs.Lock()
	defer fs.Unlock()

	_, found := fs.registry[name]
	if found {
		klog.Warningf("Filter %q has already registered", name)
		return
	}

	fs.registry[name] = fn
	fs.names = append(fs.names, name)
}

func (fs *Filters) Enabled(name string) bool {
	if fs.disabledFilters.Len() == 1 && fs.disabledFilters.Has("*") {
		return false
	}

	return !fs.disabledFilters.Has(name)
}

type Initializers []filter.Initializer

func (fis Initializers) Initialize(ins filter.ObjectFilter) error {
	for _, fi := range fis {
		if err := fi.Initialize(ins); err != nil {
			return err
		}
	}

	return nil
}
