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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappoverrider/config"
)

type YurtAppOverriderControllerOptions struct {
	*config.YurtAppOverriderControllerConfiguration
}

func NewYurtAppOverriderControllerOptions() *YurtAppOverriderControllerOptions {
	return &YurtAppOverriderControllerOptions{
		&config.YurtAppOverriderControllerConfiguration{
			ConcurrentYurtAppOverriderWorkers: 3,
		},
	}
}

// AddFlags adds flags related to nodePool for yurt-manager to the specified FlagSet.
func (n *YurtAppOverriderControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	//fs.BoolVar(&n.CreateDefaultPool, "create-default-pool", n.CreateDefaultPool, "Create default cloud/edge pools if indicated.")
	fs.Int32Var(&n.ConcurrentYurtAppOverriderWorkers, "concurrent-yurtappoverrider-workers", n.ConcurrentYurtAppOverriderWorkers, "The number of yurtappoverrider objects that are allowed to reconcile concurrently. Larger number = more responsive yurtappoverriders, but more CPU (and network) load")
}

// ApplyTo fills up nodePool config with options.
func (o *YurtAppOverriderControllerOptions) ApplyTo(cfg *config.YurtAppOverriderControllerConfiguration) error {
	if o == nil {
		return nil
	}
	cfg.ConcurrentYurtAppOverriderWorkers = o.ConcurrentYurtAppOverriderWorkers
	return nil
}

// Validate checks validation of YurtAppOverriderControllerOptions.
func (o *YurtAppOverriderControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
