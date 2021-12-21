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

package kubelet

import (
	"reflect"
	"strings"
	"testing"
)

func TestConstructNodeLabels(t *testing.T) {
	edgeWorkerLabel := "openyurt.io/is-edge-worker"
	testcases := map[string]struct {
		nodeLabels map[string]string
		mode       string
		result     map[string]string
	}{
		"no input node labels with cloud mode": {
			mode: "cloud",
			result: map[string]string{
				"openyurt.io/is-edge-worker": "false",
			},
		},
		"one input node labels with cloud mode": {
			nodeLabels: map[string]string{"foo": "bar"},
			mode:       "cloud",
			result: map[string]string{
				"openyurt.io/is-edge-worker": "false",
				"foo":                        "bar",
			},
		},
		"more than one input node labels with cloud mode": {
			nodeLabels: map[string]string{
				"foo":  "bar",
				"foo2": "bar2",
			},
			mode: "cloud",
			result: map[string]string{
				"openyurt.io/is-edge-worker": "false",
				"foo":                        "bar",
				"foo2":                       "bar2",
			},
		},
		"no input node labels with edge mode": {
			mode: "edge",
			result: map[string]string{
				"openyurt.io/is-edge-worker": "true",
			},
		},
		"one input node labels with edge mode": {
			nodeLabels: map[string]string{"foo": "bar"},
			mode:       "edge",
			result: map[string]string{
				"openyurt.io/is-edge-worker": "true",
				"foo":                        "bar",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			constructedLabelsStr := constructNodeLabels(tc.nodeLabels, tc.mode, edgeWorkerLabel)
			constructedLabels := make(map[string]string)
			parts := strings.Split(constructedLabelsStr, ",")
			for i := range parts {
				kv := strings.Split(parts[i], "=")
				if len(kv) == 2 {
					constructedLabels[kv[0]] = kv[1]
				}
			}

			if !reflect.DeepEqual(constructedLabels, tc.result) {
				t.Errorf("expected node labels: %v, but got %v", tc.result, constructedLabels)
			}
		})
	}

}
