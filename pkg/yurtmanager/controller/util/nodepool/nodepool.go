/*
Copyright 2025 The OpenYurt Authors.

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

package nodepool

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
)

// HasLeadersChanged checks if the leader endpoints have changed
// between the old and new leader slices.
// It returns true if the leaders have changed, otherwise false irrespective of the order of the leaders.
func HasLeadersChanged(old, new []v1beta2.Leader) bool {
	if len(old) != len(new) {
		return true
	}

	oldSet := make(map[v1beta2.Leader]struct{}, len(old))
	for i := range old {
		oldSet[old[i]] = struct{}{}
	}

	for i := range new {
		if _, ok := oldSet[new[i]]; !ok {
			return true
		}
	}

	return false
}

// HasPoolScopedMetadataChanged checks if the metadata has changed
// between the old and new.
// It returns true if the metadata has changed, otherwise false irrespective of the order of the metadata.
func HasPoolScopedMetadataChanged(old, new []metav1.GroupVersionKind) bool {
	if len(old) != len(new) {
		return true
	}

	oldSet := make(map[metav1.GroupVersionKind]struct{}, len(old))
	for i := range old {
		oldSet[old[i]] = struct{}{}
	}

	for i := range new {
		if _, ok := oldSet[new[i]]; !ok {
			return true
		}
	}

	return false
}
