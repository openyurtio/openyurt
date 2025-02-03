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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/hubleader/config"
)

type HubLeaderControllerOptions struct {
	*config.HubLeaderControllerConfiguration
}

func NewHubLeaderControllerOptions() *HubLeaderControllerOptions {
	return &HubLeaderControllerOptions{
		&config.HubLeaderControllerConfiguration{
			ConcurrentHubLeaderWorkers: 3,
		},
	}
}

// AddFlags adds flags related to hubleader for yurt-manager to the specified FlagSet.
func (h *HubLeaderControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if h == nil {
		return
	}

	fs.Int32Var(
		&h.ConcurrentHubLeaderWorkers,
		"concurrent-hubleader-workers",
		h.ConcurrentHubLeaderWorkers,
		"The number of nodepool objects that are allowed to reconcile concurrently.",
	)
}

// ApplyTo fills up hubleader config with options.
func (h *HubLeaderControllerOptions) ApplyTo(cfg *config.HubLeaderControllerConfiguration) error {
	if h == nil {
		return nil
	}

	cfg.ConcurrentHubLeaderWorkers = h.ConcurrentHubLeaderWorkers

	return nil
}

// Validate checks validation of HubLeaderControllerOptions.
func (h *HubLeaderControllerOptions) Validate() []error {
	if h == nil {
		return nil
	}
	errs := []error{}
	return errs
}
