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
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"

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

func registerLocalFilters(filters *Filters) {
	filters.Register("servicetopology", func() (filter.ObjectFilter, error) {
		return &nopObjectHandler{name: "servicetopology"}, nil
	})
	filters.Register("discardcloudservice", func() (filter.ObjectFilter, error) {
		return &nopObjectHandler{name: "discardcloudservice"}, nil
	})
	filters.Register("masterservice", func() (filter.ObjectFilter, error) {
		return &nopObjectHandler{name: "masterservice"}, nil
	})
}

type nopInitializer struct{}

func (nopInit *nopInitializer) Initialize(_ filter.ObjectFilter) error {
	return nil
}

func TestNewFromFilters(t *testing.T) {
	allFilters := []string{"masterservice", "discardcloudservice", "servicetopology"}
	testcases := map[string]struct {
		disabledFilters  []string
		generatedFilters sets.String
	}{
		"disable master service filter": {
			disabledFilters:  []string{"masterservice"},
			generatedFilters: sets.NewString(allFilters...).Delete("masterservice"),
		},
		"disable service topology filter": {
			disabledFilters:  []string{"servicetopology"},
			generatedFilters: sets.NewString(allFilters...).Delete("servicetopology"),
		},
		"disable discard cloud service filter": {
			disabledFilters:  []string{"discardcloudservice"},
			generatedFilters: sets.NewString(allFilters...).Delete("discardcloudservice"),
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			filters := NewFilters(tt.disabledFilters)
			registerLocalFilters(filters)

			runners, err := filters.NewFromFilters(&nopInitializer{})
			if err != nil {
				t.Errorf("failed to new from filters, %v", err)
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

	if err := initializers.Initialize(&nopObjectHandler{}); err != nil {
		t.Errorf("initialize error, %v", err)
	}
}
