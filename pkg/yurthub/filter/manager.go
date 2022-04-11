/*
Copyright 2022 The OpenYurt Authors.

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

import (
	"io"
	"net/http"

	"k8s.io/client-go/informers"
)

type Manager struct {
	Approver
	NameToFilter map[string]Runner
}

func NewFilterManager(sharedFactory informers.SharedInformerFactory, filters map[string]Runner) *Manager {
	m := &Manager{
		Approver:     newApprover(sharedFactory),
		NameToFilter: make(map[string]Runner),
	}

	for name, runner := range filters {
		m.NameToFilter[name] = runner
	}

	return m
}

func (m *Manager) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	filterName := m.Approver.GetFilterName(req)
	if runner, ok := m.NameToFilter[filterName]; ok {
		return runner.Filter(req, rc, stopCh)
	}
	return 0, rc, nil
}

func (m *Manager) Approve(req *http.Request) bool {
	return m.Approver.Approve(req)
}
