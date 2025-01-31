/*
Copyright 2025 The OpenYurt Authors.

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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type PoolScopeMetadatas []schema.GroupVersionResource

// String returns the string representation of the GVR slice.
func (psm *PoolScopeMetadatas) String() string {
	var result strings.Builder
	for i, gvr := range *psm {
		if i > 0 {
			result.WriteString(",")
		}

		result.WriteString(gvr.Group)
		result.WriteString("/")
		result.WriteString(gvr.Version)
		result.WriteString("/")
		result.WriteString(gvr.Resource)
	}

	return result.String()
}

// Set parses the input string and updates the PoolScopeMetadata slice.
func (psm *PoolScopeMetadatas) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		subParts := strings.Split(part, "/")
		if len(subParts) != 3 {
			return fmt.Errorf("invalid GVR format: %s, expected format is Group/Version/Resource", part)
		}

		*psm = append(*psm, schema.GroupVersionResource{
			Group:    strings.TrimSpace(subParts[0]),
			Version:  strings.TrimSpace(subParts[1]),
			Resource: strings.TrimSpace(subParts[2]),
		})
	}

	return nil
}

// Type returns the type of the flag as a string.
func (psm *PoolScopeMetadatas) Type() string {
	return "PoolScopeMetadatas"
}
