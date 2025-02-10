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

package nodepool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/nodepool"
)

func TestHasSliceChanged(t *testing.T) {
	tests := []struct {
		name     string
		old      []v1beta2.Leader
		new      []v1beta2.Leader
		expected bool
	}{
		{
			name: "old and new are the same",
			old: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			expected: false,
		},
		{
			name: "new has extra element",
			old: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
			},
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			expected: true,
		},
		{
			name: "old has extra element",
			old: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
			},
			expected: true,
		},
		{
			name: "new and old are different",
			old: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.3",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.4",
				},
			},
			expected: true,
		},

		{
			name: "old and new are the same but in different order",
			old: []v1beta2.Leader{
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
			},
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			expected: false,
		},
		{
			name: "old is nil",
			old:  nil,
			new: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			expected: true,
		},
		{
			name: "new is nil",
			old: []v1beta2.Leader{
				{
					NodeName: "node1",
					Address:  "10.0.0.1",
				},
				{
					NodeName: "node2",
					Address:  "10.0.0.2",
				},
			},
			new:      nil,
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := nodepool.HasSliceContentChanged(tc.old, tc.new)
			assert.Equal(t, tc.expected, actual)
		})
	}

}
