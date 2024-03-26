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

package base

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/initializer"
)

type nopObjectHandler struct {
	name string
}

func (noh *nopObjectHandler) Name() string {
	return noh.name
}

func (noh *nopObjectHandler) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{}
}

func (noh *nopObjectHandler) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	return obj
}

var (
	nodesNameErr = errors.New("nodes name error")
)

type nopNodesErrHandler struct {
	nopObjectHandler
	err error
}

func NewNopNodesErrHandler() filter.ObjectFilter {
	return &nopNodesErrHandler{
		err: nodesNameErr,
	}
}

func (nneh *nopNodesErrHandler) SetNodesGetterAndSynced(filter.NodesInPoolGetter, cache.InformerSynced, bool) error {
	return nneh.err
}

type nopInitializer struct{}

func (nopInit *nopInitializer) Initialize(_ filter.ObjectFilter) error {
	return nil
}

type errInitializer struct{}

func (errInit *errInitializer) Initialize(_ filter.ObjectFilter) error {
	return fmt.Errorf("error initialize")
}

func TestNewFromFilters(t *testing.T) {
	allFilters := []string{"masterservice", "discardcloudservice", "servicetopology"}
	testcases := map[string]struct {
		inputFilters     []string
		disabledFilters  []string
		initializer      filter.Initializer
		generatedFilters sets.String
		expectedErr      bool
	}{
		"disable master service filter": {
			inputFilters:     allFilters,
			disabledFilters:  []string{"masterservice"},
			generatedFilters: sets.NewString(allFilters...).Delete("masterservice"),
		},
		"disable service topology filter": {
			inputFilters:     allFilters,
			disabledFilters:  []string{"servicetopology"},
			generatedFilters: sets.NewString(allFilters...).Delete("servicetopology"),
		},
		"disable discard cloud service filter": {
			inputFilters:     allFilters,
			disabledFilters:  []string{"discardcloudservice"},
			generatedFilters: sets.NewString(allFilters...).Delete("discardcloudservice"),
		},
		"disable all filters": {
			inputFilters:     allFilters,
			disabledFilters:  []string{"*"},
			generatedFilters: sets.NewString(),
		},
		"register duplicated filters": {
			inputFilters:     append(allFilters, "servicetopology"),
			disabledFilters:  []string{},
			generatedFilters: sets.NewString(allFilters...),
		},
		"a invalid filter": {
			inputFilters:     append(allFilters, "invalidFilter"),
			disabledFilters:  []string{},
			generatedFilters: sets.NewString(),
			expectedErr:      true,
		},
		"initialize error": {
			inputFilters:     allFilters,
			disabledFilters:  []string{},
			initializer:      &errInitializer{},
			generatedFilters: sets.NewString(),
			expectedErr:      true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			filters := NewFilters(tt.disabledFilters)
			for i := range tt.inputFilters {
				filterName := tt.inputFilters[i]
				filters.Register(filterName, func() (filter.ObjectFilter, error) {
					if filterName == "invalidFilter" {
						return nil, fmt.Errorf("a invalide filter")
					}
					return &nopObjectHandler{name: filterName}, nil
				})
			}

			initializer := tt.initializer
			if initializer == nil {
				initializer = &nopInitializer{}
			}
			runners, err := filters.NewFromFilters(initializer)
			if err != nil && tt.expectedErr {
				return
			} else if err != nil && !tt.expectedErr {
				t.Errorf("failed to new from filters, %v", err)
				return
			} else if err == nil && tt.expectedErr {
				t.Errorf("expect an error, but got nil")
				return
			}

			gotRunners := sets.NewString()
			for i := range runners {
				gotRunners.Insert(runners[i].Name())
			}

			if !gotRunners.Equal(tt.generatedFilters) {
				t.Errorf("expect filters %v, but got %v", tt.generatedFilters, gotRunners)
			}
		})
	}
}

func TestInitializers(t *testing.T) {
	var initializers Initializers

	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps.openyurt.io", Version: "v1beta1", Resource: "nodepools"}: "NodePoolList",
	}
	yurtClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
	yurtFactory := dynamicinformer.NewDynamicSharedInformerFactory(yurtClient, 24*time.Hour)
	nodeInitializer := initializer.NewNodesInitializer(false, true, yurtFactory)
	initializers = append(initializers, nodeInitializer)

	testcases := map[string]struct {
		filter    filter.ObjectFilter
		resultErr error
	}{
		"initialize normally": {
			filter:    &nopObjectHandler{},
			resultErr: nil,
		},
		"initialize error": {
			filter:    NewNopNodesErrHandler(),
			resultErr: nodesNameErr,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := initializers.Initialize(tc.filter)
			if !errors.Is(err, tc.resultErr) {
				t.Errorf("initialize expect err %v, but got %v", tc.resultErr, err)
			}
		})
	}
}
