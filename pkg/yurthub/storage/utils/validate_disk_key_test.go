package utils

import (
	"testing"

	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

type testDiskKey struct {
	path string
}

func (k testDiskKey) Key() string {
	return k.path
}

func TestValidateDiskKey(t *testing.T) {
	cases := map[string]struct {
		key         storage.Key
		expectedErr bool
	}{
		"nil key": {
			key:         nil,
			expectedErr: true,
		},
		"empty key": {
			key:         testDiskKey{path: ""},
			expectedErr: true,
		},
		"valid key without leading slash": {
			key:         testDiskKey{path: "kubelet/pods.v1.core/default/nginx"},
			expectedErr: false,
		},
		"missing leading slash but invalid GVR": {
			key:         testDiskKey{path: "kubelet/pods/default/nginx"},
			expectedErr: false,
		},
		"valid object key (enhancement mode)": {
			key:         testDiskKey{path: "/kubelet/pods.v1.core/default/nginx"},
			expectedErr: false,
		},
		"valid object key (non-enhancement mode)": {
			key:         testDiskKey{path: "/kubelet/pods/default/nginx"},
			expectedErr: false,
		},
		"valid list key (enhancement mode)": {
			key:         testDiskKey{path: "/kubelet/pods.v1.core/default"},
			expectedErr: false,
		},
		"valid list key non-namespaced": {
			key:         testDiskKey{path: "/kubelet/nodes.v1.core/edge-worker"},
			expectedErr: false,
		},
		"valid resource list key": {
			key:         testDiskKey{path: "/kubelet/pods.v1.core"},
			expectedErr: false,
		},
		"empty component": {
			key:         testDiskKey{path: "//pods.v1.core/default/nginx"},
			expectedErr: true,
		},
		"empty GVR": {
			key:         testDiskKey{path: "/kubelet//default/nginx"},
			expectedErr: true,
		},
		"invalid GVR format (2 parts)": {
			key:         testDiskKey{path: "/kubelet/pods.v1/default/nginx"},
			expectedErr: true,
		},
		"empty namespace in object key": {
			key:         testDiskKey{path: "/kubelet/pods.v1.core//nginx"},
			expectedErr: true,
		},
		"empty name in object key": {
			key:         testDiskKey{path: "/kubelet/pods.v1.core/default/"},
			expectedErr: true,
		},
		"only one slash": {
			key:         testDiskKey{path: "/kubelet"},
			expectedErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateDiskKey(tc.key)
			if tc.expectedErr && err == nil {
				t.Errorf("ValidateDiskKey() expected error, got nil")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("ValidateDiskKey() unexpected error: %v", err)
			}
		})
	}
}
