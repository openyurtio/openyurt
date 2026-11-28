/*
Copyright 2026 The OpenYurt Authors.

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

package dragonfly

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManifestURL(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		image    string
		insecure []string
		want     string
		wantErr  bool
	}{
		{image: "busybox", want: "https://registry-1.docker.io/v2/library/busybox/manifests/latest"},
		{image: "busybox:1.36", want: "https://registry-1.docker.io/v2/library/busybox/manifests/1.36"},
		{image: "docker.io/foo/bar:v1", want: "https://registry-1.docker.io/v2/foo/bar/manifests/v1"},
		{image: "kind-registry:5000/demo/big:v2", insecure: []string{"kind-registry:5000"}, want: "http://kind-registry:5000/v2/demo/big/manifests/v2"},
		{image: "kind-registry:5000/demo/big:v2", want: "https://kind-registry:5000/v2/demo/big/manifests/v2"},
		{image: "ghcr.io/o/r@" + digest, want: "https://ghcr.io/v2/o/r/manifests/" + digest},
		{image: "ghcr.io/o/r:v1@" + digest, want: "https://ghcr.io/v2/o/r/manifests/" + digest},
		{image: "foo/UPPER", wantErr: true},
		{image: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.image, func(t *testing.T) {
			got, err := ManifestURL(tc.image, tc.insecure)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
