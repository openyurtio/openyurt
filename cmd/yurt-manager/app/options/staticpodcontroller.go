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

package options

import (
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/controller/staticpod/config"
)

const DefaultUpgradeWorkerImage = "openyurt/node-servant:latest"

type StaticPodControllerOptions struct {
	*config.StaticPodControllerConfiguration
}

func NewStaticPodControllerOptions() *StaticPodControllerOptions {
	return &StaticPodControllerOptions{
		&config.StaticPodControllerConfiguration{
			UpgradeWorkerImage: DefaultUpgradeWorkerImage,
		},
	}
}

// AddFlags adds flags related to staticpod for yurt-manager to the specified FlagSet.
func (o *StaticPodControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.UpgradeWorkerImage, "upgrade-worker-image", o.UpgradeWorkerImage, "Specify the worker pod image used for static pod upgrade.")
}

// ApplyTo fills up staticpod config with options.
func (o *StaticPodControllerOptions) ApplyTo(cfg *config.StaticPodControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.UpgradeWorkerImage = o.UpgradeWorkerImage
	return nil
}

// Validate checks validation of StaticPodControllerOptions.
func (o *StaticPodControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
