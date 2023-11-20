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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/config"
)

const DefaultUpgradeWorkerImage = "openyurt/node-servant:latest"

type YurtStaticSetControllerOptions struct {
	*config.YurtStaticSetControllerConfiguration
}

func NewYurtStaticSetControllerOptions() *YurtStaticSetControllerOptions {
	return &YurtStaticSetControllerOptions{
		&config.YurtStaticSetControllerConfiguration{
			UpgradeWorkerImage: DefaultUpgradeWorkerImage,
		},
	}
}

// AddFlags adds flags related to YurtStaticSet for yurt-manager to the specified FlagSet.
func (o *YurtStaticSetControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.UpgradeWorkerImage, "node-servant-image", o.UpgradeWorkerImage, "Specify node servant pod image used for YurtStaticSet upgrade.")
}

// ApplyTo fills up YurtStaticSet config with options.
func (o *YurtStaticSetControllerOptions) ApplyTo(cfg *config.YurtStaticSetControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.UpgradeWorkerImage = o.UpgradeWorkerImage
	return nil
}

// Validate checks validation of YurtStaticSetControllerOptions.
func (o *YurtStaticSetControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
