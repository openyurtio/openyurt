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
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/config"
)

type ImagePreheatControllerOptions struct {
	*config.ImagePreheatControllerConfiguration
}

func NewImagePreheatControllerOptions() *ImagePreheatControllerOptions {
	return &ImagePreheatControllerOptions{
		&config.ImagePreheatControllerConfiguration{
			Backend: config.BackendJob,
		},
	}
}

// AddFlags adds flags related to image preheat for yurt-manager to the specified FlagSet.
func (o *ImagePreheatControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.Backend, "image-preheat-backend", o.Backend, "The backend used to preheat images for OTA upgrade, one of: job, dragonfly.")
	fs.StringVar(&o.DragonflyManagerAddress, "dragonfly-manager-address", o.DragonflyManagerAddress, "The address of the Dragonfly manager, e.g. http://dragonfly-manager.dragonfly-system:8080. Required when --image-preheat-backend=dragonfly.")
	fs.StringVar(&o.DragonflyTokenSecret, "dragonfly-token-secret", o.DragonflyTokenSecret, "The namespace/name of the Secret that holds the Dragonfly personal access token under key \"token\". Required when --image-preheat-backend=dragonfly.")
	fs.StringSliceVar(&o.DragonflyInsecureRegistries, "dragonfly-insecure-registries", o.DragonflyInsecureRegistries, "Registry hosts that are addressed over http instead of https when preheating through Dragonfly.")
}

// ApplyTo fills up image preheat config with options.
func (o *ImagePreheatControllerOptions) ApplyTo(cfg *config.ImagePreheatControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.Backend = o.Backend
	cfg.DragonflyManagerAddress = o.DragonflyManagerAddress
	cfg.DragonflyTokenSecret = o.DragonflyTokenSecret
	cfg.DragonflyInsecureRegistries = o.DragonflyInsecureRegistries
	return nil
}

// Validate checks validation of ImagePreheatControllerOptions.
func (o *ImagePreheatControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	switch o.Backend {
	case config.BackendJob:
	case config.BackendDragonfly:
		if o.DragonflyManagerAddress == "" {
			errs = append(errs, fmt.Errorf("--dragonfly-manager-address is required when --image-preheat-backend=%s", config.BackendDragonfly))
		}
		if ns, name, ok := strings.Cut(o.DragonflyTokenSecret, "/"); !ok || ns == "" || name == "" {
			errs = append(errs, fmt.Errorf("--dragonfly-token-secret must be in namespace/name format, got %q", o.DragonflyTokenSecret))
		}
	default:
		errs = append(errs, fmt.Errorf("--image-preheat-backend must be one of %s, %s, got %q", config.BackendJob, config.BackendDragonfly, o.Backend))
	}
	return errs
}
