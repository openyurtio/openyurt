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
	"encoding/json"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestFlattenYAML(t *testing.T) {
	tests := []struct {
		name  string
		patch string
	}{
		{
			name: "test1",
			patch: `
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx
      - hello
      - world`,
		},
		{
			name: "test2",
			patch: `
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx
      - name: tomcat
        image: tomcat`}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchMap := make(map[string]interface{})
			data, err := yaml.YAMLToJSON([]byte(tt.patch))
			if err != nil {
				t.Fatalf("fail to convert patch from yaml to json: %v", err)
			}
			if err := json.Unmarshal(data, &patchMap); err != nil {
				t.Fatalf("fail to call function Unmarshal: %v", err)
			}
			if _, err := FlattenYAML(patchMap, "", "/"); err != nil {
				t.Fatalf("fail to call function FlattenYAML: %v", err)
			}
		})
	}
}
