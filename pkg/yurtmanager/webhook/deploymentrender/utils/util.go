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

package utils

import (
	"strconv"
	"strings"
)

func FlattenYAML(data map[string]interface{}, parentKey string, sep string) (map[string]interface{}, error) {
	flattenedData := make(map[string]interface{})
	for key, value := range data {
		newKey := strings.Join([]string{parentKey, key}, sep)
		switch value.(type) {
		case map[string]interface{}:
			flattenedMap, err := FlattenYAML(value.(map[string]interface{}), newKey, sep)
			if err != nil {
				return flattenedData, err
			}
			for k, v := range flattenedMap {
				flattenedData[k] = v
			}
		case []interface{}:
			for i, item := range value.([]interface{}) {
				itemKey := strings.Join([]string{newKey, strconv.Itoa(i)}, sep)
				if nestedMap, ok := item.(map[string]interface{}); ok {
					nestedData, err := FlattenYAML(nestedMap, itemKey, sep)
					if err != nil {
						return flattenedData, err
					}
					for nestedKey, nestedValue := range nestedData {
						flattenedData[nestedKey] = nestedValue
					}
				} else {
					flattenedData[itemKey] = item
				}
			}
		default:
			flattenedData[newKey] = value
		}

	}
	return flattenedData, nil
}
