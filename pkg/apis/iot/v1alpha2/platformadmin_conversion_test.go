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

package v1alpha2

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
)

func TestPlatformAdminConvertFrom(t *testing.T) {
	cases := map[string]struct {
		src            *v1beta1.PlatformAdmin
		wantErr        bool
		wantPoolName   string
		wantAdditional []string
	}{
		"multiple nodepools with nil annotations": {
			src: &v1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sample",
				},
				Spec: v1beta1.PlatformAdminSpec{
					NodePools: []string{"nodepool-a", "nodepool-b", "nodepool-c"},
				},
			},
			wantErr:        false,
			wantPoolName:   "nodepool-a",
			wantAdditional: []string{"nodepool-b", "nodepool-c"},
		},
		"multiple nodepools with existing annotations": {
			src: &v1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sample",
					Annotations: map[string]string{
						"existing": "value",
					},
				},
				Spec: v1beta1.PlatformAdminSpec{
					NodePools: []string{"nodepool-a", "nodepool-b"},
				},
			},
			wantErr:        false,
			wantPoolName:   "nodepool-a",
			wantAdditional: []string{"nodepool-b"},
		},
		"single nodepool with nil annotations": {
			src: &v1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sample",
				},
				Spec: v1beta1.PlatformAdminSpec{
					NodePools: []string{"nodepool-a"},
				},
			},
			wantErr:      false,
			wantPoolName: "nodepool-a",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dst := &PlatformAdmin{}
			err := dst.ConvertFrom(tc.src)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if dst.Spec.PoolName != tc.wantPoolName {
				t.Errorf("PoolName = %q, want %q", dst.Spec.PoolName, tc.wantPoolName)
			}

			if len(tc.wantAdditional) == 0 {
				return
			}

			raw, ok := dst.Annotations["AdditionalNodepools"]
			if !ok {
				t.Fatalf("expected AdditionalNodepools annotation to be set")
			}

			var got []string
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("failed to unmarshal AdditionalNodepools annotation: %v", err)
			}

			if len(got) != len(tc.wantAdditional) {
				t.Fatalf("AdditionalNodepools = %v, want %v", got, tc.wantAdditional)
			}
			for i := range got {
				if got[i] != tc.wantAdditional[i] {
					t.Errorf("AdditionalNodepools[%d] = %q, want %q", i, got[i], tc.wantAdditional[i])
				}
			}
		})
	}
}
