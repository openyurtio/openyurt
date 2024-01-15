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
			UpgradeWorkerImage:             DefaultUpgradeWorkerImage,
			ConcurrentYurtStaticSetWorkers: 3,
		},
	}
}

// AddFlags adds flags related to YurtStaticSet for yurt-manager to the specified FlagSet.
func (o *YurtStaticSetControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.UpgradeWorkerImage, "node-servant-image", o.UpgradeWorkerImage, "Specify node servant pod image used for YurtStaticSet upgrade.")
	fs.Int32Var(&o.ConcurrentYurtStaticSetWorkers, "concurrent-yurtstaticset-workers", o.ConcurrentYurtStaticSetWorkers, "The number of yurtstaticset objects that are allowed to reconcile concurrently. Larger number = more responsive yurtstaticsets, but more CPU (and network) load")
}

// ApplyTo fills up YurtStaticSet config with options.
func (o *YurtStaticSetControllerOptions) ApplyTo(cfg *config.YurtStaticSetControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.UpgradeWorkerImage = o.UpgradeWorkerImage
	cfg.ConcurrentYurtStaticSetWorkers = o.ConcurrentYurtStaticSetWorkers
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
