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

package options

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/config"
)

func TestImagePreheatOptions_Validate(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(o *ImagePreheatControllerOptions)
		wantErrs int
		contains string
	}{
		{name: "default", mutate: func(o *ImagePreheatControllerOptions) {}},
		{name: "unknown backend", mutate: func(o *ImagePreheatControllerOptions) { o.Backend = "foo" }, wantErrs: 1, contains: "image-preheat-backend"},
		{name: "dragonfly without address", mutate: func(o *ImagePreheatControllerOptions) {
			o.Backend = config.BackendDragonfly
			o.DragonflyTokenSecret = "dragonfly-system/df-token"
		}, wantErrs: 1, contains: "dragonfly-manager-address"},
		{name: "dragonfly bad secret", mutate: func(o *ImagePreheatControllerOptions) {
			o.Backend = config.BackendDragonfly
			o.DragonflyManagerAddress = "http://dfm:8080"
			o.DragonflyTokenSecret = "bad"
		}, wantErrs: 1, contains: "namespace/name"},
		{name: "dragonfly valid", mutate: func(o *ImagePreheatControllerOptions) {
			o.Backend = config.BackendDragonfly
			o.DragonflyManagerAddress = "http://dfm:8080"
			o.DragonflyTokenSecret = "dragonfly-system/df-token"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := NewImagePreheatControllerOptions()
			tc.mutate(o)
			errs := o.Validate()
			assert.Len(t, errs, tc.wantErrs)
			if tc.contains != "" && len(errs) > 0 {
				assert.True(t, strings.Contains(errs[0].Error(), tc.contains), errs[0].Error())
			}
		})
	}
}

func TestImagePreheatOptions_DefaultBackendIsJob(t *testing.T) {
	assert.Equal(t, config.BackendJob, NewImagePreheatControllerOptions().Backend)
}

func TestImagePreheatOptions_ApplyTo(t *testing.T) {
	o := NewImagePreheatControllerOptions()
	o.Backend = config.BackendDragonfly
	o.DragonflyManagerAddress = "http://dfm:8080"
	o.DragonflyTokenSecret = "ns/name"
	o.DragonflyInsecureRegistries = []string{"a:5000"}
	cfg := &config.ImagePreheatControllerConfiguration{}
	assert.NoError(t, o.ApplyTo(cfg))
	assert.Equal(t, *o.ImagePreheatControllerConfiguration, *cfg)
}

func TestImagePreheatOptions_AddFlags(t *testing.T) {
	o := NewImagePreheatControllerOptions()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	o.AddFlags(fs)
	err := fs.Parse([]string{
		"--image-preheat-backend=dragonfly",
		"--dragonfly-manager-address=http://dfm:8080",
		"--dragonfly-token-secret=ns/name",
		"--dragonfly-insecure-registries=a:5000,b",
	})
	assert.NoError(t, err)
	assert.Equal(t, config.BackendDragonfly, o.Backend)
	assert.Equal(t, "http://dfm:8080", o.DragonflyManagerAddress)
	assert.Equal(t, "ns/name", o.DragonflyTokenSecret)
	assert.Equal(t, []string{"a:5000", "b"}, o.DragonflyInsecureRegistries)
}
