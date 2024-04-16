/*
Copyright 2024 The OpenYurt Authors.

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

package loadbalancerset

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	aggregateKeyPrefix = "aggregated."
)

func aggregatedMaps(elems ...map[string]string) map[string]string {
	aggregatedSet := mergeMapListToMapSet(elems...)

	return mapSetToMapString(aggregatedSet)
}

func mergeMapListToMapSet(elems ...map[string]string) map[string]sets.String {
	aggregatedSet := make(map[string]sets.String)

	for _, elem := range elems {
		for key, value := range elem {
			aggregatedKey := aggregateKeyPrefix + key
			if _, exists := aggregatedSet[aggregatedKey]; !exists {
				aggregatedSet[aggregatedKey] = sets.NewString()
			}
			aggregatedSet[aggregatedKey].Insert(value)
		}
	}
	return aggregatedSet
}

func mapSetToMapString(aggregatedSet map[string]sets.String) map[string]string {
	result := make(map[string]string)
	for key, value := range aggregatedSet {
		result[key] = strings.Join(value.List(), ",")
	}
	return result
}

func filterIgnoredKeys(m map[string]string) map[string]string {
	newMap := make(map[string]string)
	for key, value := range m {
		if !strings.HasPrefix(key, aggregateKeyPrefix) {
			continue
		}
		newMap[key] = value
	}
	return newMap
}

func mergeAndPurgeMap(currentMap, desiredMap map[string]string) map[string]string {
	result := make(map[string]string)

	for key, value := range currentMap {
		if !strings.HasPrefix(key, aggregateKeyPrefix) {
			result[key] = value
		}
	}

	for key, value := range desiredMap {
		result[key] = value
	}

	return result
}

func mapValueIsEqual(oldMap, newMap map[string]string, key string) bool {
	var oldValue string
	if oldMap != nil {
		oldValue = oldMap[key]
	}

	var newValue string
	if newMap != nil {
		newValue = newMap[key]
	}

	return oldValue == newValue
}
