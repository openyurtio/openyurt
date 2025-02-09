package nodepool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/nodepool"
)

func TestHasLeadersChanged(t *testing.T) {
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
			actual := nodepool.HasLeadersChanged(tc.old, tc.new)
			assert.Equal(t, tc.expected, actual)
		})
	}

}

func TestHasPoolScopedMetadataChanged(t *testing.T) {
	tests := []struct {
		name     string
		old      []metav1.GroupVersionKind
		new      []metav1.GroupVersionKind
		expected bool
	}{
		{
			name: "old and new are the same",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			expected: false,
		},
		{
			name: "old and new are the same but in different order",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
			},
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			expected: false,
		},
		{
			name: "old has extra elements",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
			},
			expected: true,
		},
		{
			name: "new has extra elements",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			expected: true,
		},
		{
			name: "pld is nil",
			old:  nil,
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			expected: true,
		},
		{
			name: "new is nil",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			new:      nil,
			expected: true,
		},
		{
			name: "old and new are the different",
			old: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v1",
					Kind:    "StatefulSet",
				},
			},
			new: []metav1.GroupVersionKind{
				{
					Group:   "apps",
					Version: "v2",
					Kind:    "Deployment",
				},
				{
					Group:   "apps",
					Version: "v3",
					Kind:    "StatefulSet",
				},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := nodepool.HasPoolScopedMetadataChanged(tc.old, tc.new)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
