/*
Copyright 2025 The OpenYurt Authors.

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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleaderrbac/config"
)

type HubLeaderRBACControllerOptions struct {
	*config.HubLeaderRBACControllerConfiguration
}

func NewHubLeaderRBACControllerOptions() *HubLeaderRBACControllerOptions {
	return &HubLeaderRBACControllerOptions{
		&config.HubLeaderRBACControllerConfiguration{
			ConcurrentHubLeaderRBACWorkers: 1,
		},
	}
}

// AddFlags adds flags related to hubleader for yurt-manager to the specified FlagSet.
func (h *HubLeaderRBACControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if h == nil {
		return
	}

	fs.Int32Var(
		&h.ConcurrentHubLeaderRBACWorkers,
		"concurrent-hubleaderrbac-workers",
		h.ConcurrentHubLeaderRBACWorkers,
		"The number of nodepool objects that are allowed to reconcile concurrently.",
	)
}

// ApplyTo fills up hubleader config with options.
func (h *HubLeaderRBACControllerOptions) ApplyTo(cfg *config.HubLeaderRBACControllerConfiguration) error {
	if h == nil {
		return nil
	}

	cfg.ConcurrentHubLeaderRBACWorkers = h.ConcurrentHubLeaderRBACWorkers

	return nil
}

// Validate checks validation of HubLeaderControllerOptions.
func (h *HubLeaderRBACControllerOptions) Validate() []error {
	if h == nil {
		return nil
	}
	errs := []error{}
	return errs
}
