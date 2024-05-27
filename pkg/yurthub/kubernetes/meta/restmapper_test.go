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

package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openyurtio/openyurt/pkg/yurthub/util/fs"
)

var rootDir = "/tmp/restmapper"
var fsOperator fs.FileSystemOperator

// TODO:
// add test for setup to ensure the file be created

func TestCreateRESTMapperManager(t *testing.T) {
	defer func() {
		if err := os.RemoveAll(rootDir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", rootDir, err)
		}
	}()

	// initialize an empty DynamicRESTMapper
	yurtHubRESTMapperManager, err := NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to initialize an empty dynamicRESTMapper, %v", err)
	}

	// reset yurtHubRESTMapperManager
	if err := yurtHubRESTMapperManager.ResetRESTMapper(); err != nil {
		t.Fatalf("failed to reset yurtHubRESTMapperManager , %v", err)
	}

	// initialize an Non-empty DynamicRESTMapper
	// pre-cache the CRD information to the hard disk
	cachedDynamicRESTMapper := map[string]string{
		"samplecontroller.k8s.io/v1alpha1/foo":  "Foo",
		"samplecontroller.k8s.io/v1alpha1/foos": "Foo",
	}
	d, err := json.Marshal(cachedDynamicRESTMapper)
	if err != nil {
		t.Errorf("failed to serialize dynamicRESTMapper, %v", err)
	}
	err = fsOperator.CreateFile(filepath.Join(rootDir, CacheDynamicRESTMapperKey), d)
	if err != nil {
		t.Fatalf("failed to stored dynamicRESTMapper, %v", err)
	}

	// Determine whether the restmapper in the memory is the same as the information written to the disk
	yurtHubRESTMapperManager, err = NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to initialize an empty dynamicRESTMapper, %v", err)
	}
	// get the CRD information in memory
	m := yurtHubRESTMapperManager.dynamicRESTMapper
	gotMapper := dynamicRESTMapperToString(m)

	if !compareDynamicRESTMapper(gotMapper, cachedDynamicRESTMapper) {
		t.Errorf("Got mapper: %v, expect mapper: %v", gotMapper, cachedDynamicRESTMapper)
	}

	if err := yurtHubRESTMapperManager.ResetRESTMapper(); err != nil {
		t.Fatalf("failed to reset yurtHubRESTMapperManager , %v", err)
	}
}

func TestUpdateRESTMapper(t *testing.T) {
	defer func() {
		if err := os.RemoveAll(rootDir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", rootDir, err)
		}
	}()
	yurtHubRESTMapperManager, err := NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to initialize an empty dynamicRESTMapper, %v", err)
	}
	testcases := map[string]struct {
		cachedCRD        []schema.GroupVersionKind
		addCRD           schema.GroupVersionKind
		deleteCRD        schema.GroupVersionResource
		expectRESTMapper map[string]string
	}{
		"add the first CRD": {
			cachedCRD: []schema.GroupVersionKind{},
			addCRD:    schema.GroupVersionKind{Group: "samplecontroller.k8s.io", Version: "v1alpha1", Kind: "Foo"},
			expectRESTMapper: map[string]string{
				"samplecontroller.k8s.io/v1alpha1/foo":  "Foo",
				"samplecontroller.k8s.io/v1alpha1/foos": "Foo",
			},
		},

		"update with another CRD": {
			cachedCRD: []schema.GroupVersionKind{{Group: "samplecontroller.k8s.io", Version: "v1alpha1", Kind: "Foo"}},
			addCRD:    schema.GroupVersionKind{Group: "stable.example.com", Version: "v1", Kind: "CronTab"},
			expectRESTMapper: map[string]string{
				"samplecontroller.k8s.io/v1alpha1/foo":  "Foo",
				"samplecontroller.k8s.io/v1alpha1/foos": "Foo",
				"stable.example.com/v1/crontab":         "CronTab",
				"stable.example.com/v1/crontabs":        "CronTab",
			},
		},
		"delete one CRD": {
			cachedCRD: []schema.GroupVersionKind{
				{Group: "samplecontroller.k8s.io", Version: "v1alpha1", Kind: "Foo"},
				{Group: "stable.example.com", Version: "v1", Kind: "CronTab"},
			},
			deleteCRD: schema.GroupVersionResource{Group: "stable.example.com", Version: "v1", Resource: "crontabs"},
			expectRESTMapper: map[string]string{
				"samplecontroller.k8s.io/v1alpha1/foo":  "Foo",
				"samplecontroller.k8s.io/v1alpha1/foos": "Foo",
			},
		},
	}
	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			// initialize the cache CRD
			for _, gvk := range tt.cachedCRD {
				err := yurtHubRESTMapperManager.UpdateKind(gvk)
				if err != nil {
					t.Errorf("failed to initialize the restmapper, %v", err)
				}
			}
			// add CRD information
			if !tt.addCRD.Empty() {
				err := yurtHubRESTMapperManager.UpdateKind(tt.addCRD)
				if err != nil {
					t.Errorf("failed to add CRD information, %v", err)
				}
			} else {
				// delete CRD information
				err := yurtHubRESTMapperManager.DeleteKindFor(tt.deleteCRD)
				if err != nil {
					t.Errorf("failed to delete CRD information, %v", err)
				}
			}

			// verify the CRD information in memory
			m := yurtHubRESTMapperManager.dynamicRESTMapper
			memoryMapper := dynamicRESTMapperToString(m)

			if !compareDynamicRESTMapper(memoryMapper, tt.expectRESTMapper) {
				t.Errorf("Got mapper: %v, expect mapper: %v", memoryMapper, tt.expectRESTMapper)
			}

			// verify the CRD information in disk
			b, err := fsOperator.Read(filepath.Join(rootDir, CacheDynamicRESTMapperKey))
			if err != nil {
				t.Fatalf("failed to get cached CRD information, %v", err)
			}
			cacheMapper := make(map[string]string)
			err = json.Unmarshal(b, &cacheMapper)
			if err != nil {
				t.Errorf("failed to decode the cached dynamicRESTMapper, %v", err)
			}

			if !compareDynamicRESTMapper(cacheMapper, tt.expectRESTMapper) {
				t.Errorf("cached mapper: %v, expect mapper: %v", cacheMapper, tt.expectRESTMapper)
			}
		})
	}
	if err := yurtHubRESTMapperManager.ResetRESTMapper(); err != nil {
		t.Fatalf("failed to reset yurtHubRESTMapperManager , %v", err)
	}
}

func TestResetRESTMapper(t *testing.T) {
	defer func() {
		if err := os.RemoveAll(rootDir); err != nil {
			t.Errorf("Unable to clean up test directory %q: %v", rootDir, err)
		}
	}()
	// initialize the RESTMapperManager
	yurtHubRESTMapperManager, err := NewRESTMapperManager(rootDir)
	if err != nil {
		t.Errorf("failed to initialize an empty dynamicRESTMapper, %v", err)
	}
	err = yurtHubRESTMapperManager.UpdateKind(schema.GroupVersionKind{Group: "stable.example.com", Version: "v1", Kind: "CronTab"})
	if err != nil {
		t.Errorf("failed to initialize the restmapper, %v", err)
	}

	// reset the RESTMapperManager
	if err := yurtHubRESTMapperManager.ResetRESTMapper(); err != nil {
		t.Errorf("failed to reset the restmapper, %v", err)
	}

	// Verify reset result
	if len(yurtHubRESTMapperManager.dynamicRESTMapper) != 0 {
		t.Error("The cached GVR/GVK information in memory is not cleaned up.")
	} else if _, err := os.Stat(filepath.Join(rootDir, CacheDynamicRESTMapperKey)); !os.IsNotExist(err) {
		t.Error("The cached GVR/GVK information in disk is not deleted.")
	}
}

func compareDynamicRESTMapper(gotMapper map[string]string, expectedMapper map[string]string) bool {
	if len(gotMapper) != len(expectedMapper) {
		return false
	}

	for gvr, kind := range gotMapper {
		k, exists := expectedMapper[gvr]
		if !exists || k != kind {
			return false
		}
	}

	return true
}

func dynamicRESTMapperToString(m map[schema.GroupVersionResource]schema.GroupVersionKind) map[string]string {
	resultMapper := make(map[string]string, len(m))
	for currResource, currKind := range m {
		//key: Group/Version/Resource, value: Kind
		k := strings.Join([]string{currResource.Group, currResource.Version, currResource.Resource}, SepForGVR)
		resultMapper[k] = currKind.Kind
	}
	return resultMapper
}

func TestKindToResource(t *testing.T) {
	testCases := []struct {
		Kind             string
		Plural, Singular string
	}{
		{Kind: "Pod", Plural: "pods", Singular: "pod"},

		{Kind: "Gateway", Plural: "gateways", Singular: "gateway"},

		{Kind: "ReplicationController", Plural: "replicationcontrollers", Singular: "replicationcontroller"},

		// Add "ies" when ending with "y"
		{Kind: "ImageRepository", Plural: "imagerepositories", Singular: "imagerepository"},
		// Add "es" when ending with "s"
		{Kind: "miss", Plural: "misses", Singular: "miss"},
		// Add "s" otherwise
		{Kind: "lowercase", Plural: "lowercases", Singular: "lowercase"},
	}
	for i, testCase := range testCases {
		version := schema.GroupVersion{}

		plural, singular := specifiedKindToResource(version.WithKind(testCase.Kind))
		if singular != version.WithResource(testCase.Singular) || plural != version.WithResource(testCase.Plural) {
			t.Errorf("%d: unexpected plural and singular: %v %v", i, plural, singular)
		}
	}
}
