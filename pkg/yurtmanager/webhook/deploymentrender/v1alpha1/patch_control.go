/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

type PatchControl struct {
	patches     []v1alpha1.Patch
	patchObject interface{}
	// data structure
	dataStruct interface{}
}

type overrider struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// implement json patch
func (pc *PatchControl) jsonMergePatch(patches []v1alpha1.Patch) error {
	// convert into json patch format
	var patchOperations []overrider
	for _, patch := range patches {
		single := overrider{
			Op:    string(patch.Operation),
			Path:  patch.Path,
			Value: patch.Value,
		}
		patchOperations = append(patchOperations, single)
	}
	patchBytes, err := json.Marshal(patchOperations)
	if err != nil {
		return err
	}
	patchedData, err := json.Marshal(pc.patchObject.(*appsv1.Deployment))
	if err != nil {
		return err
	}
	// conduct json patch
	patchObj, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return err
	}
	patchedData, err = patchObj.Apply(patchedData)
	if err != nil {
		return err
	}
	return json.Unmarshal(patchedData, &pc.patchObject)
}

func (pc *PatchControl) updatePatches() error {
	if err := pc.jsonMergePatch(pc.patches); err != nil {
		return err
	}
	return nil
}
