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
	"fmt"
	"slices"

	"github.com/distribution/reference"
)

const (
	dockerHubDomain   = "docker.io"
	dockerHubRegistry = "registry-1.docker.io"
	defaultTag        = "latest"
)

// ManifestURL converts a container image reference into the registry manifest url
// that Dragonfly expects for image preheating, e.g.
// busybox:1.36 -> https://registry-1.docker.io/v2/library/busybox/manifests/1.36.
// Registries listed in insecureRegistries are addressed over http.
func ManifestURL(image string, insecureRegistries []string) (string, error) {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", fmt.Errorf("parse image %q: %w", image, err)
	}

	domain := reference.Domain(named)
	host := domain
	if domain == dockerHubDomain {
		host = dockerHubRegistry
	}

	scheme := "https"
	if slices.Contains(insecureRegistries, domain) || slices.Contains(insecureRegistries, host) {
		scheme = "http"
	}

	ref := defaultTag
	if digested, ok := named.(reference.Digested); ok {
		ref = digested.Digest().String()
	} else if tagged, ok := named.(reference.Tagged); ok {
		ref = tagged.Tag()
	}

	return fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, host, reference.Path(named), ref), nil
}
