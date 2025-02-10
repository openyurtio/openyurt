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

// HasSliceContentChanged checks if the content of the old and new slices has changed.
func HasSliceContentChanged[T comparable](old, new []T) bool {
	if len(old) != len(new) {
		return true
	}

	oldSet := make(map[T]struct{}, len(old))
	for _, v := range old {
		oldSet[v] = struct{}{}
	}

	for _, v := range new {
		if _, ok := oldSet[v]; !ok {
			return true
		}
	}

	return false
}
